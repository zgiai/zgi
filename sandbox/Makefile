SHELL := /bin/sh

.PHONY: fmt test build check run kest test-linux-secure check-config-docs profile-build-stdlib
.PHONY: docker-up docker-down docker-logs
.PHONY: docker-config

fmt:
	gofmt -w $$(find . -name '*.go')

test:
	go test ./...

build:
	go build ./...

check: fmt test build

run:
	go run ./cmd/server

kest:
	.kest/run-sandbox-flows.sh

test-linux-secure:
	./scripts/test_linux_secure_container.sh

check-config-docs:
	./scripts/check_config_docs

profile-build-stdlib:
	tmp="$$(mktemp -d /tmp/zgi-profile-build.XXXXXX)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	./scripts/build-profile --profile stdlib --output-dir "$$tmp"

docker-up:
	./scripts/start-docker

docker-config:
	POSTGRES_PASSWORD=zgi-sandbox-local docker compose --env-file .env.example config >/dev/null

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f sandbox postgres redis
