# ============================================================
# Stage 1: Build the Go binary
# ============================================================
FROM golang:1.26.2-bookworm AS builder

WORKDIR /src

ARG BUILD_REGION=global
ARG GOPROXY=

ENV GOPROXY=${GOPROXY}

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/runner ./cmd/executor

# ============================================================
# Stage 2: Runtime image
# ============================================================
FROM debian:bookworm-slim

WORKDIR /app

ARG BUILD_REGION=global
ARG APT_MIRROR=
ARG APT_SECURITY_MIRROR=
ARG NPM_REGISTRY=
ARG PIP_INDEX_URL=
ARG UV_INDEX_URL=
ARG UV_VERSION=0.10.6

# Install system dependencies in a single layer
RUN if [ -n "$APT_MIRROR" ]; then \
      sed -i "s|http://deb.debian.org/debian|http://$APT_MIRROR/debian|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
    && if [ -n "$APT_SECURITY_MIRROR" ]; then \
      sed -i "s|http://deb.debian.org/debian-security|http://$APT_SECURITY_MIRROR/debian-security|g" /etc/apt/sources.list.d/debian.sources; \
    fi
RUN apt-get update -o Acquire::Retries=5 \
    && apt-get install -y --no-install-recommends -o Acquire::Retries=5 \
    ca-certificates \
    curl \
    git \
    python3 \
    python3-pip \
    python3-venv \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/python3 /usr/bin/python

RUN if [ -n "$PIP_INDEX_URL" ]; then \
      pip3 config set global.index-url "$PIP_INDEX_URL"; \
    fi \
    && if [ -n "$NPM_REGISTRY" ]; then \
      npm config set registry "$NPM_REGISTRY"; \
    fi

# Upgrade pip
RUN pip3 install --no-cache-dir --upgrade pip --break-system-packages --retries 5 --timeout 120

# Install uv through pip to avoid flaky runtime installer script dependency.
ENV UV_INDEX_URL=${UV_INDEX_URL}
RUN pip3 install --no-cache-dir --break-system-packages --retries 5 --timeout 120 "uv==${UV_VERSION}" \
    && command -v uv >/dev/null

# Copy the built binary from builder stage
COPY --from=builder /out/runner /app/runner
RUN chmod +x /app/runner

# Verify installations
RUN python3 --version && pip3 --version && uv --version && node --version && npm --version

# Create directories for plugin runtime
RUN mkdir -p /app/plugins /app/workspace /app/cache

# ============================================================
# Default environment variables
# These can be overridden by Zeabur environment variables
# ============================================================
ENV EXECUTOR_HTTP_PORT=2665 \
    EXECUTOR_PLUGIN_HOME=/app/plugins \
    EXECUTOR_WORKSPACE_PATH=/app/workspace \
    EXECUTOR_PACKAGE_CACHE_PATH=/app/cache \
    EXECUTOR_PYTHON_INTERPRETER=python3 \
    EXECUTOR_NODE_INTERPRETER=node \
    EXECUTOR_PIP_COMMAND=pip3 \
    EXECUTOR_NPM_COMMAND=npm \
    EXECUTOR_UV_PATH=/usr/local/bin/uv

# ============================================================
# External services (configured via Zeabur environment variables)
# ============================================================
# Database (PostgreSQL/MySQL):
#   EXECUTOR_DB_DRIVER=postgres
#   EXECUTOR_DB_HOST=<your-db-host>
#   EXECUTOR_DB_PORT=5432
#   EXECUTOR_DB_USER=<your-db-user>
#   EXECUTOR_DB_PASSWORD=<your-db-password>
#   EXECUTOR_DB_NAME=<your-db-name>
#   EXECUTOR_DB_SSL_MODE=require  (for production)
#
# Redis:
#   EXECUTOR_REDIS_HOST=<your-redis-host>
#   EXECUTOR_REDIS_PORT=6379
#   EXECUTOR_REDIS_PASSWORD=<your-redis-password>
#   EXECUTOR_REDIS_USE_TLS=true  (for production)
#
# Security:
#   EXECUTOR_API_KEY=<your-api-key>
# ============================================================

EXPOSE 2665

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:2665/healthz || exit 1

ENTRYPOINT ["/app/runner"]
