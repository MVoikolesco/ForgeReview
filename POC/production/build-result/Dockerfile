FROM alpine:3.20

RUN apk add --no-cache ca-certificates su-exec \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --chmod=0755 server /app/server
COPY web /app/web
COPY prompts /app/prompts
COPY app/config /app/config
COPY --chmod=0755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN test -x /app/server \
    && test -f /app/web/index.html \
    && test -f /app/config/review-prompts.yaml \
    && mkdir -p /data \
    && chown -R app:app /app /data

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/app/server"]
