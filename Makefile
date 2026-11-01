.PHONY: generate migrate migrate-down run test docker-build docker-run

-include .env
export

generate:
	go tool oapi-codegen -config oapi-codegen.yaml -o api/api.gen.go contracts/openapi/trip-service.openapi.yaml

migrate:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	go tool goose -dir migrations postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/trip-service

test:
	go test -race ./...

docker-build:
	docker build -f deploy/Dockerfile -t trip-service .

docker-run:
	docker run --rm --network host --env-file .env trip-service
