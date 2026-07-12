.PHONY: infra dev test vet docker-up docker-down

infra:
	docker compose up -d postgres redis

dev: infra
	air

test:
	go test ./...

vet:
	go vet ./...

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
