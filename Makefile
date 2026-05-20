.DEFAULT_GOAL := help

.PHONY: help bootstrap setup status env-check env-sync install-hooks check-open-source dev-api dev-web dev-docker docker-down docker-logs

help:
	@echo "Available commands:"
	@echo "  make bootstrap   Prepare env files and docker compose"
	@echo "  make setup       Bootstrap repository and install local dependencies"
	@echo "  make status      Show root and service status"
	@echo "  make env-check   Compare env files against their templates"
	@echo "  make env-sync    Backup env files and append missing template keys"
	@echo "  make install-hooks Install repository Git hooks"
	@echo "  make check-open-source Run open-source hygiene checks"
	@echo "  make dev-docker  Build and start the full local docker stack"
	@echo "                    Tip: ./dev/start-docker --china uses China mainland build mirrors"
	@echo "  make docker-down Stop the local docker stack"
	@echo "  make docker-logs Tail logs from the local docker stack"
	@echo "  make dev-api     Start backend from api/"
	@echo "  make dev-web     Start frontend from web/"

bootstrap:
	@./dev/bootstrap

setup:
	@./dev/setup

status:
	@./dev/status

env-check:
	@./dev/check-env

env-sync:
	@./dev/check-env --sync

install-hooks:
	@./dev/install-git-hooks

check-open-source:
	@./scripts/check-open-source.sh --worktree

dev-api:
	@./dev/start-api

dev-web:
	@./dev/start-web

dev-docker:
	@./dev/start-docker

docker-down:
	@docker compose -f docker/docker-compose.yaml --env-file docker/.env down

docker-logs:
	@docker compose -f docker/docker-compose.yaml --env-file docker/.env logs -f
