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

# The security source goes first: the main source URI is a prefix of it, so
# replacing the main source first would also take over the security source.
RUN if [ -n "$APT_SECURITY_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian-security|http://$APT_SECURITY_MIRROR/debian-security|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    if [ -n "$APT_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian|http://$APT_MIRROR/debian|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing --no-install-recommends bubblewrap util-linux && \
    sed 's/\r$//' profiles/skill-office/apt-packages.txt | grep -v '^[[:space:]]*$' | xargs -r apt-get -o Acquire::Retries=5 install -y --fix-missing --no-install-recommends && \
    rm -rf /var/lib/apt/lists/*

ENV PATH=/usr/bin:/bin:/usr/local/bin:/usr/sbin:/sbin

ARG NPM_CONFIG_REGISTRY=
ARG PIP_INDEX_URL=

RUN if [ -n "$NPM_CONFIG_REGISTRY" ]; then npm config set registry "$NPM_CONFIG_REGISTRY"; fi && \
    if [ -n "$PIP_INDEX_URL" ]; then export PIP_INDEX_URL; fi && \
    python3 -c "import ssl; print(ssl.OPENSSL_VERSION)" && \
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
    mkdir -p /out/skill-office/dev /out/skill-office/proc /out/skill-office/run /out/skill-office/sys && \
    mkdir -p /out/skill-office/opt/zgi/profiles && \
    mv /tmp/profile-build/skill-office /out/skill-office/opt/zgi/profiles/skill-office

FROM python:3.12-slim-bookworm AS scientific-data-profile

WORKDIR /src

COPY --from=builder /out/zgi-sandbox-profilebuilder /usr/local/bin/zgi-sandbox-profilebuilder

ARG PIP_INDEX_URL=
ARG APT_MIRROR=
ARG APT_SECURITY_MIRROR=

# The security source goes first: the main source URI is a prefix of it, so
# replacing the main source first would also take over the security source.
RUN if [ -n "$APT_SECURITY_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian-security|http://$APT_SECURITY_MIRROR/debian-security|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    if [ -n "$APT_MIRROR" ]; then \
        sed -i "s|http://deb.debian.org/debian|http://$APT_MIRROR/debian|g" /etc/apt/sources.list.d/*.sources; \
    fi && \
    apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --fix-missing --no-install-recommends \
        bubblewrap util-linux \
        r-base-core r-base-dev \
        r-bioc-edger r-bioc-deseq2 r-bioc-sva \
        ca-certificates curl libgmp-dev \
        r-cran-knitr r-cran-rmarkdown r-cran-plotly r-cran-ggplot2 \
        r-cran-robustbase r-cran-matrix r-cran-quantreg r-cran-rjson \
        r-cran-glmnet r-cran-numderiv r-cran-rcpp r-cran-gmp r-cran-iterators && \
    curl -fsSL https://cloud.r-project.org/src/contrib/arrangements_1.1.10.tar.gz -o /tmp/arrangements.tar.gz && \
    echo 'db564c30dcd42b36f3098a2dd522bbdea239213a5c42fa3a9a48453fce72780e  /tmp/arrangements.tar.gz' | sha256sum -c - && \
    R CMD INSTALL /tmp/arrangements.tar.gz && \
    curl -fsSL https://cloud.r-project.org/src/contrib/iterpc_0.4.2.tar.gz -o /tmp/iterpc.tar.gz && \
    echo '38bd464042a27536f676e889263eb2c257a431b59083f58cb54473f42ba2071b  /tmp/iterpc.tar.gz' | sha256sum -c - && \
    R CMD INSTALL /tmp/iterpc.tar.gz && \
    curl -fsSL https://cloud.r-project.org/src/contrib/MendelianRandomization_0.10.0.tar.gz -o /tmp/mendelian-randomization.tar.gz && \
    echo '0851e91f826424f20fd4a58348ffe161d147bdc091d24d676e14d4cd6180e13c  /tmp/mendelian-randomization.tar.gz' | sha256sum -c - && \
    R CMD INSTALL /tmp/mendelian-randomization.tar.gz && \
    rm -f /tmp/arrangements.tar.gz /tmp/iterpc.tar.gz /tmp/mendelian-randomization.tar.gz && \
    rm -rf /var/lib/apt/lists/*

COPY profiles ./profiles

RUN --mount=type=cache,target=/root/.cache/pip \
    if [ -n "$PIP_INDEX_URL" ]; then export PIP_INDEX_URL; fi && \
    zgi-sandbox-profilebuilder --profile scientific-data --source-dir profiles --output-dir /tmp/profile-build --force && \
    mkdir -p /out/scientific-data && \
    tar --one-file-system \
        --exclude=/dev --exclude=./dev \
        --exclude=/out --exclude=./out \
        --exclude=/proc --exclude=./proc \
        --exclude=/run --exclude=./run \
        --exclude=/src --exclude=./src \
        --exclude=/sys --exclude=./sys \
        --exclude=/tmp/profile-build --exclude=./tmp/profile-build \
        -C / -cf - . | tar -C /out/scientific-data -xf - && \
    mkdir -p /out/scientific-data/dev /out/scientific-data/proc /out/scientific-data/run /out/scientific-data/sys && \
    mkdir -p /out/scientific-data/opt/zgi/profiles && \
    mv /tmp/profile-build/scientific-data /out/scientific-data/opt/zgi/profiles/scientific-data

FROM python:3.11-slim-bookworm

WORKDIR /app

# The server stays non-root. Bubblewrap itself needs its distro-supported
# setuid mode to create the inner user/network namespaces when the image is
# run inside a container; it immediately maps the analysis process to uid/gid
# 65534 as enforced by secure_bwrap.go.
RUN apt-get -o Acquire::Retries=5 update && \
    apt-get -o Acquire::Retries=5 install -y --no-install-recommends bubblewrap util-linux && \
    rm -rf /var/lib/apt/lists/* && \
    useradd --system --create-home --home-dir /home/zgi --shell /usr/sbin/nologin zgi && \
    chmod u+s /usr/bin/bwrap && \
    mkdir -p /var/lib/zgi-sandbox/dependency-rootfs && \
    chown -R zgi:zgi /var/lib/zgi-sandbox

# Copy each rootfs once, with its final ownership. Copying /out and then
# recursively chowning it in another layer duplicates multi-gigabyte profiles
# in the image and can exhaust disk space during export.
COPY --from=node-runtime /usr/local/ /usr/local/
COPY --from=builder /out/zgi-sandbox /usr/local/bin/zgi-sandbox
COPY --from=builder /out/zgi-sandbox-profilebuilder /usr/local/bin/zgi-sandbox-profilebuilder
COPY --from=system-office-profile --chown=zgi:zgi /out/skill-office /opt/zgi/prebuilt-dependency-rootfs/skill-office
COPY --from=scientific-data-profile --chown=zgi:zgi /out/scientific-data /opt/zgi/prebuilt-dependency-rootfs/scientific-data
COPY --chmod=755 scripts/build-dependency-profile /usr/local/bin/zgi-sandbox-build-dependency-profile
COPY --chmod=755 scripts/docker-entrypoint /usr/local/bin/zgi-sandbox-docker-entrypoint
RUN sed -i 's/\r$//' /usr/local/bin/zgi-sandbox-build-dependency-profile /usr/local/bin/zgi-sandbox-docker-entrypoint

EXPOSE 2660

USER zgi

ENTRYPOINT ["zgi-sandbox-docker-entrypoint"]
CMD ["zgi-sandbox"]
