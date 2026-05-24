# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.26.2-alpine AS builder

ARG BUILD_REGION=global
ARG GOPROXY=

ENV GOPROXY=${GOPROXY}

# Set working directory
WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build application
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o server cmd/server/main.go

# Runtime stage
FROM alpine:latest

ARG BUILD_REGION=global
ARG APK_MIRROR=

RUN if [ -n "$APK_MIRROR" ]; then \
      sed -i "s|https\?://dl-cdn.alpinelinux.org/alpine|https://$APK_MIRROR/alpine|g" /etc/apk/repositories; \
    fi

# Install runtime tools.
# poppler-utils supplies pdftocairo/pdftoppm for PDF rasterization; fontconfig
# and core fonts provide Helvetica-compatible fallbacks for PDF text rendering.
# tesseract handles self-hosted OCR fallback for scan/image-like documents.
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    bash \
    wget \
    su-exec \
    fontconfig \
    ttf-dejavu \
    font-liberation \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-data-eng \
    tesseract-ocr-data-chi_sim

# Set timezone
ENV TZ=Asia/Shanghai

# Create non-root user
RUN adduser -D -g '' appuser

# Create required directories
RUN mkdir -p /app/config /app/logs /app/templates

# Set working directory
WORKDIR /app

# Copy binary and optional files from build stage (before switching user)
COPY --from=builder /app/server .
COPY --from=builder /app/config ./config/
COPY --from=builder /app/templates ./templates/
COPY --from=builder /app/internal/modules/skills/catalog ./internal/modules/skills/catalog/

# Copy entrypoint script
COPY --chmod=755 docker/entrypoint.sh /app/entrypoint.sh
RUN sed -i 's/\r$//' /app/entrypoint.sh

# Set file ownership for appuser
RUN chown -R appuser:appuser /app

# Expose port
EXPOSE 2670

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=40s --retries=3 \
  CMD wget --no-verbose --tries=1 -O /dev/null "http://localhost:${SERVER_PORT:-2670}/ping" || exit 1

ENTRYPOINT ["/bin/sh", "/app/entrypoint.sh"]
