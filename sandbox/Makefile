SHELL := /bin/sh

.PHONY: fmt test build check run kest test-linux-secure test-agent-runtime-containerd test-agent-runtime-e2b check-docker-entrypoint check-config-docs profile-build-stdlib profile-build-scientific-data build-agent-runtime-image build-agent-runtime-worker
.PHONY: docker-up docker-down docker-logs
.PHONY: docker-config

fmt:
	gofmt -w $$(find . -name '*.go')

test:
	go test ./...

build:
	go build ./...

check: fmt test build check-docker-entrypoint

run:
	go run ./cmd/server

kest:
	.kest/run-sandbox-flows.sh

test-linux-secure:
	./scripts/test_linux_secure_container.sh

test-agent-runtime-containerd:
	go test -tags=agent_runtime_integration -count=1 -v ./internal/localruntime/containerd -run TestContainerdGVisorContract

test-agent-runtime-e2b:
	go test -tags=e2b_integration -count=1 -v ./internal/hostedruntime/e2b -run TestE2BContract

check-docker-entrypoint:
	./scripts/test_docker_entrypoint

check-config-docs:
	./scripts/check_config_docs

profile-build-stdlib:
	tmp="$$(mktemp -d /tmp/zgi-profile-build.XXXXXX)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	./scripts/build-profile --profile stdlib --output-dir "$$tmp"

profile-build-scientific-data:
	tmp="$$(mktemp -d /tmp/zgi-scientific-data-profile.XXXXXX)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	./scripts/build-profile --profile scientific-data --output-dir "$$tmp"

build-agent-runtime-image:
	docker build -f Dockerfile.agent-runtime -t zgi-agent-runtime:dev .

build-agent-runtime-worker:
	CGO_ENABLED=0 GOOS=linux go build -trimpath -o bin/zgi-agent-runtime-worker ./cmd/agentworker

docker-up:
	./scripts/start-docker

docker-config:
	POSTGRES_PASSWORD=zgi-sandbox-local docker compose --env-file .env.example config >/dev/null

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f sandbox postgres redis
