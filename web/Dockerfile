# syntax=docker/dockerfile:1

# ============================================================
# ZGI Web Platform - Multi-stage Docker Build
# ============================================================

# --- Base Stage ---
FROM node:20-alpine3.20 AS base

# China mirror (uncomment if needed)
# RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

RUN apk add --no-cache tzdata wget bash
RUN --mount=type=cache,target=/root/.npm \
    npm install -g pnpm@10.12.1

ENV PNPM_HOME="/pnpm"
ENV PNPM_STORE_DIR="/pnpm/store"
ENV PATH="$PNPM_HOME:$PATH"


# --- Dependencies Stage ---
FROM base AS deps
WORKDIR /app/web

COPY package.json pnpm-lock.yaml ./

# China registry (uncomment if needed)
# RUN pnpm install --frozen-lockfile --registry https://registry.npmmirror.com/
RUN --mount=type=cache,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store \
    && pnpm install --frozen-lockfile


# --- Builder Stage ---
FROM base AS builder
WORKDIR /app/web

COPY --from=deps /app/web/node_modules ./node_modules
COPY . .

ARG BASE_PATH
ARG NEXT_PUBLIC_BASE_PATH
ARG NEXT_PUBLIC_APP_NAME
ARG NEXT_PUBLIC_BRAND_NAME
ARG NEXT_PUBLIC_LOGO_URL
ARG NEXT_PUBLIC_DARK_LOGO_URL
ARG NEXT_PUBLIC_FAVICON_URL
ARG NEXT_PUBLIC_ICON_TEXT
ARG NEXT_PUBLIC_IS_CLOUD
ARG NEXT_BUILD_CPUS=2
ARG NEXT_STATIC_GENERATION_MAX_CONCURRENCY=2
ARG NEXT_TURBOPACK_MEMORY_LIMIT_MB=2048
ARG NEXT_BUILD_NODE_MEMORY_MB=2048
ARG NEXT_WEBPACK_MEMORY_OPTIMIZATIONS=true
ARG CUSTOMER_PRESERVE_GENERATED
ARG CUSTOMER_REQUIRED
ENV BASE_PATH=${BASE_PATH}
ENV NEXT_PUBLIC_BASE_PATH=${NEXT_PUBLIC_BASE_PATH}
ENV NEXT_PUBLIC_APP_NAME=${NEXT_PUBLIC_APP_NAME}
ENV NEXT_PUBLIC_BRAND_NAME=${NEXT_PUBLIC_BRAND_NAME}
ENV NEXT_PUBLIC_LOGO_URL=${NEXT_PUBLIC_LOGO_URL}
ENV NEXT_PUBLIC_DARK_LOGO_URL=${NEXT_PUBLIC_DARK_LOGO_URL}
ENV NEXT_PUBLIC_FAVICON_URL=${NEXT_PUBLIC_FAVICON_URL}
ENV NEXT_PUBLIC_ICON_TEXT=${NEXT_PUBLIC_ICON_TEXT}
ENV NEXT_PUBLIC_IS_CLOUD=${NEXT_PUBLIC_IS_CLOUD}
ENV NEXT_BUILD_CPUS=${NEXT_BUILD_CPUS}
ENV NEXT_STATIC_GENERATION_MAX_CONCURRENCY=${NEXT_STATIC_GENERATION_MAX_CONCURRENCY}
ENV NEXT_TURBOPACK_MEMORY_LIMIT_MB=${NEXT_TURBOPACK_MEMORY_LIMIT_MB}
ENV NEXT_WEBPACK_MEMORY_OPTIMIZATIONS=${NEXT_WEBPACK_MEMORY_OPTIMIZATIONS}
ENV CUSTOMER_PRESERVE_GENERATED=${CUSTOMER_PRESERVE_GENERATED}
ENV CUSTOMER_REQUIRED=${CUSTOMER_REQUIRED}
ENV NODE_OPTIONS="--max-old-space-size=${NEXT_BUILD_NODE_MEMORY_MB}"
RUN --mount=type=cache,target=/app/web/.next/cache \
    if [ -z "$NEXT_PUBLIC_BASE_PATH" ] && [ -n "$BASE_PATH" ]; then export NEXT_PUBLIC_BASE_PATH="$BASE_PATH"; fi; pnpm build


# --- Production Stage ---
FROM node:20-alpine3.20 AS production

# Install minimal runtime dependencies
RUN apk add --no-cache tzdata bash \
    && npm install -g pm2

# Environment defaults
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    TZ=Asia/Shanghai \
    PM2_INSTANCES=2

# Set timezone
RUN ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime \
    && echo ${TZ} > /etc/timezone

WORKDIR /app/web

# Copy build artifacts
COPY --from=builder /app/web/public ./public
COPY --from=builder /app/web/.next/standalone ./
COPY --from=builder /app/web/.next/static ./.next/static
COPY docker/entrypoint.sh ./entrypoint.sh

# Setup permissions for non-root user
RUN mkdir -p /.pm2 \
    && chown -R 1001:0 /.pm2 /app/web \
    && chmod -R g=u /.pm2 /app/web \
    && sed -i 's/\r$//' ./entrypoint.sh \
    && chmod +x ./entrypoint.sh

# Build metadata
ARG COMMIT_SHA
ENV COMMIT_SHA=${COMMIT_SHA}

USER 1001
EXPOSE 2680

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD node -e "const port=process.env.PORT||2680;const bp=(process.env.NEXT_PUBLIC_BASE_PATH||process.env.BASE_PATH||'').replace(/\/+$/,'');const p=(bp?bp:'')+'/env.js';require('http').get('http://127.0.0.1:'+port+p,res=>process.exit(res.statusCode===200?0:1)).on('error',()=>process.exit(1))"

ENTRYPOINT ["./entrypoint.sh"]
