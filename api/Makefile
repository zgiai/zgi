.PHONY: all build run test clean swagger migrate seed seed-prod seed-list generate air docker-up docker-down docker-logs

# Build executable
build:
	go build -o bin/server cmd/server/main.go

# Run service
run:
	go run cmd/server/main.go start

# Run with hot reload using air
air:
	air -c .air.toml

# Run tests
test:
	go test -v ./...

# Generate swagger documentation
swagger:
	swag init -g cmd/server/main.go -o docs

# Database migration (Note: Please create PostgreSQL database first: CREATE DATABASE zgi_ginkit;)
migrate:
	go run cmd/migrate/main.go up

seed:
	go run cmd/migrate/main.go seed
# Clean build files
clean:
	rm -rf bin/
	rm -rf docs/

# Download dependencies
deps:
	go mod download

# Update dependencies
deps-update:
	go get -u ./...

# Code formatting
fmt:
	go fmt ./...

# Code checking
lint:
	golangci-lint run

# Docker related commands
docker-build:
	docker build -t zgi-ginkit .

docker-run:
	docker run -p 8080:8080 zgi-ginkit

docker-up:
	./scripts/start-docker

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f zgi-api postgres redis neo4j weaviate

# Generate CRUD code
generate:
	@echo "Generate CRUD code"
	@echo "Usage: make generate model=ModelName [table=table_name] [package=package_name] [output=output_dir] [force=true]"
	@if [ "$(model)" = "" ]; then \
		echo "Error: Model name must be specified (model=ModelName)"; \
		exit 1; \
	fi
	go run cmd/generator/main.go -model $(model) $(if $(table),-table $(table)) $(if $(package),-package $(package)) $(if $(output),-output $(output)) $(if $(force),-force)
