SHELL := /bin/sh

.PHONY: fmt test build check run test-linux-secure
.PHONY: docker-up docker-down docker-logs

fmt:
	gofmt -w $$(find . -name '*.go')

test:
	go test ./...

build:
	go build ./...

check: fmt test build

run:
	go run ./cmd/server

test-linux-secure:
	./scripts/test_linux_secure_container.sh

docker-up:
	./scripts/start-docker

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f sandbox postgres redis
