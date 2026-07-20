FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -o /app/bin/server ./cmd/server

FROM node:22-alpine AS web-builder
WORKDIR /web-admin
COPY web-admin/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web-admin ./
ENV NEXT_OUTPUT=export
RUN npm run build

FROM golang:1.23-alpine AS development

WORKDIR /app

RUN apk add --no-cache ca-certificates su-exec \
    && go install github.com/air-verse/air@v1.60.0 \
    && addgroup -S app \
    && adduser -S app -G app \
    && mkdir -p /tmp/forgereview-air /data /logs/diffs \
    && chown -R app:app /tmp/forgereview-air /data /logs

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
    && chown -R app:app /app

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/go/bin/air", "-c", ".air.toml"]

FROM alpine:3.20

RUN apk add --no-cache ca-certificates su-exec \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/prompts /app/prompts
COPY --from=builder /app/config /app/config
COPY --from=web-builder /web-admin/out /app/web
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
    && mkdir -p /data /logs/diffs \
    && chown -R app:app /app /data /logs

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/app/server"]
