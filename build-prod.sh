#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROD_DIR="${ROOT_DIR}/Prod"
COMPOSE_FILE="${PROD_DIR}/compose.yaml"
ENV_FILE="${PROD_DIR}/.env"
ENV_STAGE="${PROD_DIR}/.env.build"
WEB_DIR="${ROOT_DIR}/web-admin"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'Erro: %s nao encontrado no PATH.\n' "$1" >&2
    exit 1
  fi
}

for command in go node npm; do
  require_command "${command}"
done

for path in \
  "${ROOT_DIR}/cmd/server" \
  "${ROOT_DIR}/prompts" \
  "${ROOT_DIR}/config/review-prompts.yaml" \
  "${WEB_DIR}/package.json" \
  "${WEB_DIR}/package-lock.json" \
  "${ROOT_DIR}/docker-entrypoint.sh"; do
  if [[ ! -e "${path}" ]]; then
    printf 'Erro: caminho obrigatorio nao encontrado: %s\n' "${path}" >&2
    exit 1
  fi
done

mkdir -p "${PROD_DIR}"

current_tag=0
if [[ -f "${COMPOSE_FILE}" ]]; then
  current_tag="$(sed -nE 's/^[[:space:]]*image:[[:space:]]*forgereview-runtime:local-v([0-9]+)[[:space:]]*$/\1/p' "${COMPOSE_FILE}" | sort -n | tail -n 1)"
  current_tag="${current_tag:-0}"
fi
next_tag=$((current_tag + 1))

if [[ -f "${ENV_FILE}" ]]; then
  cp "${ENV_FILE}" "${ENV_STAGE}"
elif [[ -f "${ROOT_DIR}/.env" ]]; then
  cp "${ROOT_DIR}/.env" "${ENV_STAGE}"
else
  printf 'Erro: nenhum .env encontrado em Prod/.env ou na raiz do projeto.\n' >&2
  printf 'Crie o .env de producao antes de executar o build.\n' >&2
  exit 1
fi

cleanup() {
  rm -f "${ENV_STAGE}"
}
trap cleanup EXIT

rm -rf \
  "${PROD_DIR}/app" \
  "${PROD_DIR}/prompts" \
  "${PROD_DIR}/web" \
  "${PROD_DIR}/server" \
  "${PROD_DIR}/Dockerfile" \
  "${PROD_DIR}/docker-entrypoint.sh"

mkdir -p "${PROD_DIR}/app/config" "${PROD_DIR}/prompts" "${PROD_DIR}/web"

printf 'Compilando binario Linux estatico...\n'
(
  cd "${ROOT_DIR}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" go build -o "${PROD_DIR}/server.tmp" ./cmd/server
)
mv "${PROD_DIR}/server.tmp" "${PROD_DIR}/server"
chmod 0755 "${PROD_DIR}/server"

printf 'Compilando painel Next.js estatico...\n'
(
  cd "${WEB_DIR}"
  npm ci --no-audit --no-fund
  npm run lint
  NEXT_TELEMETRY_DISABLED=1 npm run build
)

if [[ ! -f "${WEB_DIR}/out/index.html" ]]; then
  printf 'Erro: o build do frontend nao gerou %s.\n' "${WEB_DIR}/out/index.html" >&2
  exit 1
fi

printf 'Sincronizando artefatos da aplicacao...\n'
cp -R "${ROOT_DIR}/prompts/." "${PROD_DIR}/prompts/"
cp "${ROOT_DIR}/config/review-prompts.yaml" "${PROD_DIR}/app/config/review-prompts.yaml"
chmod -R a+rX "${PROD_DIR}/prompts" "${PROD_DIR}/app/config"
cp -R "${WEB_DIR}/out/." "${PROD_DIR}/web/"
cp "${ROOT_DIR}/docker-entrypoint.sh" "${PROD_DIR}/docker-entrypoint.sh"
chmod 0755 "${PROD_DIR}/docker-entrypoint.sh"

mkdir -p "${PROD_DIR}/data"

cat > "${PROD_DIR}/Dockerfile" <<'EOF'
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
EOF

cat > "${COMPOSE_FILE}" <<EOF
services:
  redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

  api:
    image: forgereview-runtime:local-v${next_tag}
    build: .
    restart: unless-stopped
    env_file:
      - .env
    environment:
      APP_MODE: api
      PORT: 8080
      DATABASE_DSN: /data/forgereview.db
      REDIS_ADDR: redis:6379
      REVIEW_PROMPT_CONFIG_PATH: /app/config/review-prompts.yaml
    ports:
      - "8088:8080"
    volumes:
      - ./data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 5s
      timeout: 3s
      retries: 20
    depends_on:
      redis:
        condition: service_healthy

  worker:
    image: forgereview-runtime:local-v${next_tag}
    build: .
    restart: unless-stopped
    env_file:
      - .env
    environment:
      APP_MODE: worker
      DATABASE_DSN: /data/forgereview.db
      REDIS_ADDR: redis:6379
      REDIS_CONSUMER: worker-1
      REVIEW_PROMPT_CONFIG_PATH: /app/config/review-prompts.yaml
    volumes:
      - ./data:/data
    depends_on:
      redis:
        condition: service_healthy
      api:
        condition: service_healthy

volumes:
  redis_data:
EOF

mv -f "${ENV_STAGE}" "${ENV_FILE}"
trap - EXIT

if command -v docker >/dev/null 2>&1; then
  docker compose -f "${COMPOSE_FILE}" config --quiet
else
  printf 'Aviso: Docker nao encontrado; validacao do Compose foi ignorada.\n'
fi

printf '\nPasta Prod recriada com sucesso.\n'
printf 'Imagem: forgereview-runtime:local-v%s\n' "${next_tag}"
printf 'Frontend: %s\n' "${PROD_DIR}/web"
printf 'Binario: %s\n' "${PROD_DIR}/server"
printf 'Env preservado em: %s\n' "${ENV_FILE}"
printf 'Dados persistentes: %s\n' "${PROD_DIR}/data"
printf 'Proximo passo: copie a pasta Prod para o servidor e execute docker compose up -d --build.\n'
