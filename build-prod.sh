#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROD_DIR="${ROOT_DIR}/Prod"
COMPOSE_FILE="${PROD_DIR}/compose.yaml"
ENV_FILE="${PROD_DIR}/.env"

command -v go >/dev/null 2>&1 || {
  printf 'Erro: Go nao encontrado no PATH.\n' >&2
  exit 1
}

for path in "${ROOT_DIR}/cmd/server" "${ROOT_DIR}/prompts" "${ROOT_DIR}/config/review-prompts.yaml"; do
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
  cp "${ENV_FILE}" "${PROD_DIR}/.env.build-backup"
elif [[ -f "${ROOT_DIR}/.env" ]]; then
  cp "${ROOT_DIR}/.env" "${PROD_DIR}/.env.build-backup"
else
  printf 'Erro: nenhum .env encontrado em Prod/.env ou na raiz do projeto.\n' >&2
  printf 'Crie o .env de producao antes de executar o build.\n' >&2
  exit 1
fi

printf 'Compilando binario Linux estatico...\n'
(
  cd "${ROOT_DIR}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH:-amd64}" go build -o "${PROD_DIR}/server" ./cmd/server
)
chmod 0755 "${PROD_DIR}/server"

printf 'Sincronizando prompts e configuracao...\n'
rm -rf "${PROD_DIR}/prompts" "${PROD_DIR}/app/config"
mkdir -p "${PROD_DIR}/prompts" "${PROD_DIR}/app/config"
cp -R "${ROOT_DIR}/prompts/." "${PROD_DIR}/prompts/"
cp "${ROOT_DIR}/config/review-prompts.yaml" "${PROD_DIR}/app/config/review-prompts.yaml"

cat > "${PROD_DIR}/Dockerfile" <<'EOF'
FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --chmod=0755 server /usr/local/bin/gitea-review
COPY prompts /app/prompts
COPY app/config /app/config

RUN test -x /usr/local/bin/gitea-review \
    && mkdir -p /logs/diffs \
    && chown -R app:app /app /logs

USER app

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/gitea-review"]
EOF

cat > "${COMPOSE_FILE}" <<EOF
services:
  redis:
    image: redis:7-alpine
    restart: unless-stopped
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data

  api:
    image: forgereview-runtime:local-v${next_tag}
    build: .
    restart: unless-stopped
    env_file:
      - .env
    environment:
      APP_MODE: api
      PORT: 8080
      REDIS_ADDR: redis:6379
      REVIEW_PROMPT_CONFIG_PATH: /app/config/review-prompts.yaml
    ports:
      - 8088:8080
    depends_on:
      - redis

  worker:
    image: forgereview-runtime:local-v${next_tag}
    build: .
    restart: unless-stopped
    user: 0:0
    env_file:
      - .env
    environment:
      APP_MODE: worker
      REDIS_ADDR: redis:6379
      REDIS_CONSUMER: worker-1
      DIFF_LOG_DIR: /logs/diffs
      REVIEW_PROMPT_CONFIG_PATH: /app/config/review-prompts.yaml
    volumes:
      - /opt/stacks/gitea-review/logs:/logs
      - /opt/stacks/gitea-review/prompts:/app/prompts:ro
      - /opt/stacks/gitea-review/app/config:/app/config:ro
    depends_on:
      - redis

volumes:
  redis_data:
EOF

mv "${PROD_DIR}/.env.build-backup" "${ENV_FILE}"

if command -v docker >/dev/null 2>&1; then
  docker compose -f "${COMPOSE_FILE}" config --quiet
else
  printf 'Aviso: Docker nao encontrado; validacao do Compose foi ignorada.\n'
fi

printf '\nPasta Prod recriada com sucesso.\n'
printf 'Imagem: forgereview-runtime:local-v%s\n' "${next_tag}"
printf 'Binario: %s\n' "${PROD_DIR}/server"
printf 'Env preservado em: %s\n' "${ENV_FILE}"
printf 'Proximo passo: copie a pasta Prod para o servidor e faca o build da stack no Dockge.\n'
