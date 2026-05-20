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
RUN go mod download

# Copy source code
COPY . .

# Build application
RUN CGO_ENABLED=0 GOOS=linux go build -v -o server cmd/server/main.go

# Runtime stage
FROM alpine:latest

ARG BUILD_REGION=global
ARG APK_MIRROR=

RUN if [ -n "$APK_MIRROR" ]; then \
      sed -i "s|https\?://dl-cdn.alpinelinux.org/alpine|https://$APK_MIRROR/alpine|g" /etc/apk/repositories; \
    fi

# Install runtime tools.
# poppler-utils supplies pdftoppm for PDF rasterization; tesseract handles
# self-hosted OCR fallback for scan/image-like documents.
RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    bash \
    wget \
    su-exec \
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
  CMD wget --no-verbose --tries=1 -O /dev/null http://localhost:2670/ping || exit 1

ENTRYPOINT ["/bin/sh", "/app/entrypoint.sh"]
