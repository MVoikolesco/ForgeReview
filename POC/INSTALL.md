# Instalação do ForgeReview

## Requisitos

- Docker Engine com Docker Compose v2;
- uma instância Gitea acessível pelos containers;
- Ollama acessível pela rede **ou** uma API key do OpenRouter.

## Instalação

```sh
cp .env.example .env
docker compose -f docker/compose.yml up -d --build
docker compose -f docker/compose.yml ps
```

No `.env`, configure apenas bootstrap, infraestrutura e segredos:

```env
GITEA_BOT_USERNAME=ia-reviewer
GITEA_URL=https://gitea.example
GITEA_TOKEN=...
ADMIN_USERNAME=admin
ADMIN_PASSWORD=troque-esta-senha
GITEA_TOKEN_ENCRYPTION_KEY=uma-chave-com-exatos-32-bytes
APP_ENVIRONMENT=production
```

O Compose define internamente o caminho correto do SQLite (`/data/forgereview.db`) e compartilha o volume `config_data` entre API e worker.

Abra `http://localhost:3000`, faça login e escolha **Configurar IA**. O assistente valida a conexão, consulta o catálogo do provider e grava conexão, modelo, parâmetros, profile e policy. A API permanece disponível em `http://localhost:8088` para integrações e fallback estático; os prompts internos versionados pelo projeto não são modificados pelo painel.

Para desenvolvimento, use `APP_ENVIRONMENT=development`; o serviço web sobe com `npm run dev`. O valor padrão `production` compila o Next e sobe com `npm run start`.

Para Ollama executando na máquina host, use esta URL na conexão cadastrada:

```text
http://host.docker.internal:11434
```

Para OpenRouter ou Ollama Cloud, informe a chave diretamente no assistente. Ela é cifrada no SQLite, não aparece novamente e API/worker devem receber a mesma `GITEA_TOKEN_ENCRYPTION_KEY`.

A área **Operação** exibe heartbeat dos workers, tamanho e pendências da fila, reviews com artefatos e o log consolidado do worker.

## Diagnóstico

```sh
docker compose -f docker/compose.yml ps
docker compose -f docker/compose.yml logs --tail=100 api worker web
```

Todos os serviços devem aparecer como ativos e API/Redis como saudáveis. Para recriar containers sem apagar o SQLite:

```sh
docker compose -f docker/compose.yml down
docker compose -f docker/compose.yml up -d --build --force-recreate
```

Não acrescente `-v` ao comando `down` se quiser preservar as configurações.
