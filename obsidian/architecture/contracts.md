# Contratos preservados

- `POST /webhook` aceita eventos `review_requested` e ignora outros eventos ou reviewers diferentes de `GITEA_BOT_USERNAME`.
- `POST /review` aceita URL no formato `/owner/repository/pulls/number`.
- `/api/admin/*` usa Basic Auth e respostas legadas (`[]` para listas, `{error: ...}` em falhas).
- Resultado final preserva `comments[].file`, `line`, `severity`, `decision_reason`, `comment` e `final_review`.
- Steps versionados expõem status e mensagem para o flow visual do painel.
