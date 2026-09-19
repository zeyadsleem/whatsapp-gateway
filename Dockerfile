# ---- Frontend build ---------------------------------------------------------
FROM node:24-alpine AS web

RUN corepack enable
WORKDIR /src/web/admin

COPY web/admin/package.json web/admin/pnpm-lock.yaml web/admin/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/admin/ ./
RUN pnpm build

# ---- Go build ---------------------------------------------------------------
FROM golang:1.27-alpine AS builder

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_TIME=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/web/admin/dist ./web/admin/dist

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X github.com/zeyad/whatsapp-gateway/internal/version.Version=${VERSION:-dev} \
      -X github.com/zeyad/whatsapp-gateway/internal/version.Commit=${COMMIT:-none} \
      -X github.com/zeyad/whatsapp-gateway/internal/version.BuildTime=${BUILD_TIME:-unknown}" \
    -o /out/server ./cmd/server

# ---- Runtime ----------------------------------------------------------------
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S gateway \
    && adduser -S -G gateway -H gateway

COPY --from=builder /out/server /usr/local/bin/server

USER gateway
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/server"]
