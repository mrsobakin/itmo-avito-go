.PHONY: generate migrate migrate-down run lint test test-integration docker-build docker-run

-include .env
export

generate:
	go tool oapi-codegen -config oapi-codegen.yaml -o api/api.gen.go contracts/openapi/trip-service.openapi.yaml
	go generate ./...

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

lint:
	golangci-lint run ./...

test:
	go test -race -coverprofile=coverage.out ./...
	@grep -E '^mode:|/internal/(domain|usecase)/' coverage.out > coverage.core.out
	@go tool cover -func=coverage.core.out | tail -1 | sed 's/^total:/coverage domain+usecase:/'
	@go tool cover -func=coverage.out | tail -1 | sed 's/^total:/coverage total:/'

test-integration:
	go test -race -count=1 -tags integration ./internal/repository/...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service .

docker-run:
	docker run --rm --network host --env-file .env trip-service
