# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.2-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY=

WORKDIR /src

ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/zgi-sandbox ./cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/zgi-sandbox-profilebuilder ./cmd/profilebuilder

FROM node:20-bookworm-slim AS node-runtime

FROM python:3.11-slim-bookworm AS system-office-profile

ARG APT_MIRROR=
ARG APT_SECURITY_MIRROR=

WORKDIR /src

COPY --from=node-runtime /usr/local/ /usr/local/
COPY --from=builder /out/zgi-sandbox-profilebuilder /usr/local/bin/zgi-sandbox-profilebuilder
COPY profiles ./profiles

RUN if [ -n "$APT_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian|http://$APT_MIRROR/debian|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    if [ -n "$APT_SECURITY_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian-security|http://$APT_SECURITY_MIRROR/debian-security|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing --no-install-recommends bubblewrap && \
    sed 's/\r$//' profiles/skill-office/apt-packages.txt | grep -v '^[[:space:]]*$' | xargs -r apt-get -o Acquire::Retries=5 install -y --fix-missing --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

ENV PATH=/usr/bin:/bin:/usr/local/bin:/usr/sbin:/sbin

RUN python3 -c "import ssl; print(ssl.OPENSSL_VERSION)" && \
    npm install -g pnpm@9.15.9 && \
    zgi-sandbox-profilebuilder --profile skill-office --source-dir profiles --output-dir /tmp/profile-build --force && \
    mkdir -p /out/skill-office && \
    tar --one-file-system \
        --exclude=/dev --exclude=./dev \
        --exclude=/out --exclude=./out \
        --exclude=/proc --exclude=./proc \
        --exclude=/run --exclude=./run \
        --exclude=/src --exclude=./src \
        --exclude=/sys --exclude=./sys \
        --exclude=/tmp/profile-build --exclude=./tmp/profile-build \
        -C / -cf - . | tar -C /out/skill-office -xf - && \
    mkdir -p /out/skill-office/opt/zgi/profiles && \
    mv /tmp/profile-build/skill-office /out/skill-office/opt/zgi/profiles/skill-office

FROM system-office-profile

WORKDIR /app

COPY --from=builder /out/zgi-sandbox /usr/local/bin/zgi-sandbox
COPY --chmod=755 scripts/build-dependency-profile /usr/local/bin/zgi-sandbox-build-dependency-profile
COPY --chmod=755 scripts/docker-entrypoint /usr/local/bin/zgi-sandbox-docker-entrypoint
RUN sed -i 's/\r$//' /usr/local/bin/zgi-sandbox-build-dependency-profile /usr/local/bin/zgi-sandbox-docker-entrypoint

RUN useradd --system --create-home --home-dir /home/zgi --shell /usr/sbin/nologin zgi && \
    mkdir -p /opt/zgi/prebuilt-dependency-rootfs && \
    cp -R /out/. /opt/zgi/prebuilt-dependency-rootfs/ && \
    rm -rf /out /src && \
    mkdir -p /var/lib/zgi-sandbox/dependency-rootfs && \
    chown -R zgi:zgi /opt/zgi/prebuilt-dependency-rootfs && \
    chown -R zgi:zgi /var/lib/zgi-sandbox

EXPOSE 2660

USER zgi

ENTRYPOINT ["zgi-sandbox-docker-entrypoint"]
CMD ["zgi-sandbox"]
