# Лабораторная 3

## Запуск

Для запуска нужно поднять окружение (postgres, телеметрию и заглушку Push Service), применить миграции и запустить сам сервис:

```bash
tripgoctl cluster start
tripgoctl environment start
make migrate
make run
```

Адреса Jaeger, Grafana и Prometheus печатает `tripgoctl connect`.

## Тесты и линтер

- `make lint` запускает `golangci-lint`.
- `make test` запускает unit-тесты с `-race` и печатает покрытие `domain + usecase`.
- `make test-integration` проверяет репозитории на postgres из `DATABASE_URL`, поэтому окружение `tripgoctl` должно быть поднято.
- `make generate` генерирует код по OpenAPI и моки.

Интеграционные тесты отделены билд-тегом `integration`, каждый тест создаёт и удаляет свои данные.

## Переменные окружения

Все переменные обязательные, кроме `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_RESOURCE_ATTRIBUTES` и `OTEL_TRACES_SAMPLER`.

| Переменная | Пример | Назначение |
|---|---|---|
| `HTTP_ADDR` | `:8080` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | `5s` | суммарный таймаут запроса |
| `HTTP_READ_HEADER_TIMEOUT` | `2s` | таймаут для получения хэдеров запроса |
| `HTTP_WRITE_TIMEOUT` | `10s` | таймаут ответа |
| `HTTP_IDLE_TIMEOUT` | `60s` | таймаут keep-alive соединений |
| `LOG_LEVEL` | `info` | уровень логов |
| `SHUTDOWN_TIMEOUT` | `10s` | таймаут ожидания для graceful shutdown |
| `DATABASE_URL` | `postgres://...` | URL PostgreSQL |
| `DATABASE_MAX_CONNS` | `10` | максимальный размер пула соединений |
| `DATABASE_MIN_CONNS` | `2` | минимальный размер пула |
| `DATABASE_MAX_CONN_LIFETIME` | `30m` | через сколько соединение пересоздаётся |
| `DATABASE_CONNECT_TIMEOUT` | `5s` | таймаут подключения к базе |
| `DATABASE_QUERY_TIMEOUT` | `3s` | таймаут каждого запроса к базе, в том числе `BEGIN` и `COMMIT` |
| `IDEMPOTENCY_KEY_TTL` | `24h` | время жизни ключа идемпотентности |
| `POSITION_CHECK_INTERVAL` | `30s` | период тикера сканера, он же пауза между повторными пушами |
| `POSITION_STALE_AFTER` | `60s` | через сколько без координат поездка считается устаревшей |
| `POSITION_WORKERS` | `4` | число воркеров |
| `POSITION_QUEUE_SIZE` | `100` | ёмкость очереди |
| `POSITION_SCAN_BATCH_SIZE` | `100` | размер порции выборки |
| `PUSH_HTTP_URL` | `http://localhost:8090` | адрес Push Service |
| `PUSH_REQUEST_TIMEOUT` | `1s` | таймаут одного запроса в Push Service |
| `OTEL_SERVICE_NAME` | `trip-service` | имя сервиса в телеметрии и логах |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:4318` | адрес коллектора |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` | протокол OTLP |
| `OTEL_RESOURCE_ATTRIBUTES` | `service.namespace=tripgo,...` | атрибуты ресурса |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` | сэмплер |

Таймаутов HTTP-сервера, `IDEMPOTENCY_KEY_TTL` и `POSITION_SCAN_BATCH_SIZE` нет в дефолтных значениях курса, поэтому они заданы в секции `[env]` файла `environment.toml`.

## Архитектура

```
http -------> usecase ---> domain <--- repository ---> infra/postgres
  |              |                         |
  +---> model <--+-------------------------+
container ---> все пакеты
```

- `domain/trip`: поездка, переход `Finish`, валидация и доменные ошибки. Не зависит от других слоёв.
- `usecase/trip/{create,get,finish}`: сценарии, знают только домен и свои интерфейсы.
- `http/trip/{create,get,finish}`: хендлеры. `http/domainerror` является единственным местом, где ошибка превращается в HTTP-ответ.
- `usecase/position/{save,list}`, `http/position/{save,list}`: сохранение координаты и маршрут.
- `repository/{trip,position,idempotency}`: запросы через squirrel и pgx.
- `worker/positions`: сканер, очередь и пул воркеров опроса устаревших поездок.
- `gateway/push`: HTTP-клиент Push Service.
- `infra/postgres`, `infra/telemetry`: пул, менеджер транзакций, OpenTelemetry и логгер.
- `container`: единственное место, где создаются реализации и связываются между собой.

Правила зависимостей проверяет `depguard` в `make lint`.

## Опрос устаревших поездок

```
тикер ---> сканер ---> очередь ---> воркеры ---> Push Service
             ^            ^
             |            |
            БД      POST /positions (отмена)
```

На каждом тике сканер читает из бд поездки, по которым координат не было дольше `POSITION_STALE_AFTER` (если координат не было вообще, считается от `started_at`). Чтение идёт порциями по `POSITION_SCAN_BATCH_SIZE`, от самых давно молчащих. Каждая поездка ставится в очередь неблокирующе, результат пишется в `tripgo.stale_checks{result}`. Тики не накладываются: сканер один, и следующий тик он читает только после окончания обхода.

Очередь хранит поездки, которые ждут обработки или обрабатываются прямо сейчас, поэтому одна поездка не может попасть в неё дважды. Все операции с очередью идут под одним мьютексом, и проверка дубликата со вставкой атомарны. Пришедшая координата удаляет поездку из очереди, и запрос не уходит (`tripgo.position_requests{result="cancelled"}`). Отметку об отправленной команде не храним, поэтому неответившее устройство получит повторный пуш на следующем тике, то есть не чаще раза в `POSITION_CHECK_INTERVAL`.

Когда очередь заполнена, новая поездка сравнивается с самой свежей в очереди. Если новая молчит дольше, она занимает место, а вытесненная дропается, иначе дропается сама новая. Дроп увеличивает `tripgo.stale_checks{result="dropped"}` и пишет `Warn`. Если дропнулась сама новая поездка, очередь перестала принимать, и обход на этом тике прекращается: следующие поездки в выборке ещё свежее. Поездка при этом не теряется: координат по ней всё ещё нет, и она попадёт в выборку следующего тика.

Ошибка Push Service (транспорт, `4xx`, `5xx`, таймаут) пишется в `Warn` и в `tripgo.position_requests{result="error"}`, воркер переходит к следующей задаче.

Остановка идёт в порядке: HTTP-сервер, сканер, воркеры дорабатывают очередь, HTTP-клиент и пул БД. Бюджет общий, `SHUTDOWN_TIMEOUT`. Если воркеры не успели, их контекст отменяется, текущие запросы прерываются, а оставшиеся в очереди поездки опросит следующий запуск сервиса.

## Принятые решения

### Запрос выборки и индекс

```sql
SELECT id, driver_id, GREATEST(last_position_at, started_at) FROM trips
WHERE status = 'active'
  AND GREATEST(last_position_at, started_at) < now() - make_interval(secs => $1)
  AND (GREATEST(last_position_at, started_at), id) > ($2, $3) -- со второй порции
ORDER BY GREATEST(last_position_at, started_at), id
LIMIT $4
```

```sql
CREATE INDEX CONCURRENTLY trips_active_silent_since_idx
    ON trips ((GREATEST(last_position_at, started_at)), id)
    WHERE status = 'active';
```

Индекс частичный, так как завершённых поездок со временем станет большинство, а сканер их не читает. Ключ индекса совпадает с ключом сортировки, поэтому условие по времени, продолжение порции и `ORDER BY ... LIMIT` выполняются одним index scan без сортировки. `id` в ключе нужен, чтобы поездки с одинаковым временем не терялись и не читались дважды на границе порций. `GREATEST`, а не `COALESCE`, нужен, чтобы координата с `recorded_at` раньше `started_at` (например, с отстающими часами устройства) не сделала свежую поездку устаревшей. `status = 'active'` записан литералом: с параметром планировщик не может доказать, что частичный индекс подходит.

### Сохранение координаты

Координата и `trips.last_position_at` пишутся в одной транзакции. Поездка берётся `FOR UPDATE`, чтобы координата не могла попасть в поездку, которую параллельно завершают. Опоздавшая координата не состаривает поездку, так как обновление делается в самом запросе: `SET last_position_at = GREATEST(last_position_at, $1)` (`GREATEST` игнорирует `NULL`).

## Задания со звёздочкой

### Выборка батчами

Сканер читает порции по `POSITION_SCAN_BATCH_SIZE` с продолжением по ключу `(GREATEST(last_position_at, started_at), id)`, без `OFFSET`. Порции читаются, пока не кончатся или пока очередь не перестанет принимать. Сколько строк прочитано за тик, видно в `tripgo.stale_checks.rows_read`.

### Приоритетная очередь

Вместо канала используется `container/heap` по времени, с которого поездка молчит. Воркер всегда берёт поездку, которая молчит дольше всех, и ждёт задачу на `sync.Cond`. Отмена по координате удаляет задачу из кучи.
