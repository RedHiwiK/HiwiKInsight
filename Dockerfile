# syntax=docker/dockerfile:1

# ---- dashboard (React) ----
FROM node:22-slim AS web
RUN corepack enable && corepack prepare pnpm@10.18.3 --activate
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# ---- server (Go, pure-Go SQLite, no cgo) ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist/app ./web/dist/app
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/hiwikinsight ./cmd/hiwikinsight \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/insight ./cmd/insight \
 && mkdir -p /out/data

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hiwikinsight /out/insight /usr/local/bin/
# /data holds the database; owned by the distroless "nonroot" user (65532)
COPY --from=build --chown=65532:65532 /out/data /data
WORKDIR /data
ENV HIWIKINSIGHT_CONFIG=/etc/hiwikinsight/config.yaml \
    HIWIKINSIGHT_DATA_DIR=/data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/hiwikinsight"]
CMD ["serve"]
