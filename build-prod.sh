#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROD_DIR="${ROOT_DIR}/Prod"
COMPOSE_FILE="${PROD_DIR}/compose.yaml"

command -v go >/dev/null 2>&1 || {
  printf 'Erro: Go nao encontrado no PATH.\n' >&2
  exit 1
}

for path in "${PROD_DIR}" "${COMPOSE_FILE}" "${ROOT_DIR}/prompts" "${ROOT_DIR}/config/review-prompts.yaml"; do
  if [[ ! -e "${path}" ]]; then
    printf 'Erro: caminho obrigatorio nao encontrado: %s\n' "${path}" >&2
    exit 1
  fi
done

current_tag="$(sed -nE 's/^[[:space:]]*image:[[:space:]]*forgereview-runtime:local-v([0-9]+)[[:space:]]*$/\1/p' "${COMPOSE_FILE}" | sort -n | tail -n 1)"
if [[ -z "${current_tag}" ]]; then
  printf 'Erro: nao foi encontrada uma tag forgereview-runtime:local-vN em %s\n' "${COMPOSE_FILE}" >&2
  exit 1
fi

next_tag=$((current_tag + 1))

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

printf 'Atualizando imagem para local-v%s...\n' "${next_tag}"
sed -E -i "s/(image:[[:space:]]*forgereview-runtime:local-v)[0-9]+/\1${next_tag}/g" "${COMPOSE_FILE}"

if command -v docker >/dev/null 2>&1; then
  docker compose -f "${COMPOSE_FILE}" config --quiet
else
  printf 'Aviso: Docker nao encontrado; validacao do Compose foi ignorada.\n'
fi

printf '\nPacote Prod atualizado com sucesso.\n'
printf 'Imagem: forgereview-runtime:local-v%s\n' "${next_tag}"
printf 'Binario: %s\n' "${PROD_DIR}/server"
printf 'Proximo passo: copie a pasta Prod para o servidor e faca o build da stack no Dockge.\n'
