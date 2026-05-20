FROM --platform=$BUILDPLATFORM golang:1.26.2-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_REGION=global
ARG GOPROXY=

WORKDIR /src

ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o /out/zgi-sandbox ./cmd/server

FROM --platform=$TARGETPLATFORM node:20-bookworm-slim AS node-runtime

FROM --platform=$TARGETPLATFORM python:3.11-slim-bookworm

WORKDIR /app

COPY --from=node-runtime /usr/local/ /usr/local/

COPY --from=builder /out/zgi-sandbox /usr/local/bin/zgi-sandbox

RUN useradd --system --create-home --home-dir /home/zgi --shell /usr/sbin/nologin zgi && \
    mkdir -p /var/lib/zgi-sandbox && \
    chown -R zgi:zgi /var/lib/zgi-sandbox

EXPOSE 8194

USER zgi

CMD ["zgi-sandbox"]
