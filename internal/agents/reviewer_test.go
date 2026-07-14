package agents

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea-agents/internal/gitea"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
)

func TestReviewerAgentFetchesPullRequestDiff(t *testing.T) {
	var logs bytes.Buffer
	diffLogDir := t.TempDir()
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	ollamaClient := &fakeOllamaClient{
		model: "deepseek-coder:6.7b",
		responses: []string{`review bloco 1
CONTRATOS_DECLARADOS:
- ARQUIVO: internal/app.go
  TIPO: metodo
  NOME: App.testeMethod
		REFERENCIAS_A_VERIFICAR:
- nenhum
		REGRAS_DE_VALIDACAO:
		- nenhum`, "review bloco 2", validFinalResponse()},
	}
	agent := NewReviewerAgent(log.New(&logs, "", 0), giteaClient, ollamaClient, ReviewerOptions{
		DiffLogDir:             diffLogDir,
		MaxBlockChars:          4000,
		MaxFilesPerBlock:       1,
		ReviewPromptConfigPath: testPromptConfigPath(),
	})

	job := queue.ReviewJob{
		Owner:             "Qualyagro",
		Repo:              "wiki",
		PRNumber:          12,
		RequestedReviewer: "ia-reviewer",
		Sender:            "marcio",
	}

	if err := agent.Process(context.Background(), job); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if giteaClient.owner != "Qualyagro" || giteaClient.repo != "wiki" || giteaClient.prNumber != 12 {
		t.Fatalf("unexpected gitea call: owner=%s repo=%s pr=%d", giteaClient.owner, giteaClient.repo, giteaClient.prNumber)
	}

	if !strings.Contains(logs.String(), "diff obtido owner=Qualyagro repo=wiki pr=12 size=") {
		t.Fatalf("expected diff size log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "diff parseado owner=Qualyagro repo=wiki pr=12 files=3") {
		t.Fatalf("expected parsed diff log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "arquivo alterado path=README.md additions=2 deletions=1") {
		t.Fatalf("expected README.md file log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "arquivo alterado path=internal/app.go additions=1 deletions=1") {
		t.Fatalf("expected internal/app.go file log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "arquivo alterado path=internal/service.go additions=1 deletions=1") {
		t.Fatalf("expected internal/service.go file log, got %q", logs.String())
	}

	if strings.Contains(logs.String(), "-old line") {
		t.Fatalf("expected deleted file diff content only in file, got %q", logs.String())
	}

	if strings.Contains(logs.String(), `+fmt.Println("new")`) {
		t.Fatalf("expected any file diff content only in file, got %q", logs.String())
	}

	if len(ollamaClient.prompts) != 3 {
		t.Fatalf("expected 3 ollama calls, got %d", len(ollamaClient.prompts))
	}

	if !strings.Contains(ollamaClient.prompts[0], "Este e o bloco 1 de 2") {
		t.Fatalf("expected first partial prompt, got %q", ollamaClient.prompts[0])
	}

	if !strings.Contains(ollamaClient.prompts[1], "Este e o bloco 2 de 2") {
		t.Fatalf("expected second partial prompt, got %q", ollamaClient.prompts[1])
	}

	if !strings.Contains(ollamaClient.prompts[1], "Memoria tecnica acumulada") ||
		!strings.Contains(ollamaClient.prompts[1], "App.testeMethod") {
		t.Fatalf("expected second partial prompt with accumulated memory, got %q", ollamaClient.prompts[1])
	}

	if strings.Contains(ollamaClient.prompts[0], "README.md") || strings.Contains(ollamaClient.prompts[1], "README.md") {
		t.Fatalf("expected README to be ignored in review prompts, got %q / %q", ollamaClient.prompts[0], ollamaClient.prompts[1])
	}

	if !strings.Contains(ollamaClient.prompts[2], "Reviews parciais:") ||
		!strings.Contains(ollamaClient.prompts[2], "review bloco 1") ||
		!strings.Contains(ollamaClient.prompts[2], "Memoria tecnica consolidada para cruzamento") ||
		!strings.Contains(ollamaClient.prompts[2], "App.testeMethod") {
		t.Fatalf("expected final prompt with partial reviews, got %q", ollamaClient.prompts[2])
	}

	reviewLogDir := filepath.Join(diffLogDir, "Qualyagro_wiki_pr-12")
	diffLog, err := os.ReadFile(filepath.Join(reviewLogDir, "01-diff-completo.log"))
	if err != nil {
		t.Fatalf("expected full diff log file, got %v", err)
	}

	diffLogContent := string(diffLog)
	if !strings.Contains(diffLogContent, "===== INICIO DIFF ARQUIVO 1/3 path=README.md additions=2 deletions=1 =====") {
		t.Fatalf("expected README.md diff block start, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, "===== FIM DIFF ARQUIVO 1/3 path=README.md =====") {
		t.Fatalf("expected README.md diff block end, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, "===== INICIO DIFF ARQUIVO 2/3 path=internal/app.go additions=1 deletions=1 =====") {
		t.Fatalf("expected internal/app.go diff block start, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, "===== FIM DIFF ARQUIVO 2/3 path=internal/app.go =====") {
		t.Fatalf("expected internal/app.go diff block end, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, "===== INICIO DIFF ARQUIVO 3/3 path=internal/service.go additions=1 deletions=1 =====") {
		t.Fatalf("expected internal/service.go diff block start, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, "-old line") {
		t.Fatalf("expected deleted file diff content in file, got %q", diffLogContent)
	}

	if !strings.Contains(diffLogContent, `+fmt.Println("new")`) {
		t.Fatalf("expected any file diff content in file, got %q", diffLogContent)
	}

	if !strings.Contains(logs.String(), "logs fisicos do review dir=") {
		t.Fatalf("expected physical log directory metadata, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "prompt config carregado path=") {
		t.Fatalf("expected prompt config log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "blocos de review gerados owner=Qualyagro repo=wiki pr=12 blocks=2") {
		t.Fatalf("expected blocks log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "enviando bloco para ollama block=1 total=2 files=1") || !strings.Contains(logs.String(), "timeout=900s") {
		t.Fatalf("expected sending block log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "review final gerado chars=") {
		t.Fatalf("expected final review log, got %q", logs.String())
	}

	if !giteaClient.reviewCreated {
		t.Fatal("expected final review to be published")
	}
	if giteaClient.reviewOwner != "Qualyagro" || giteaClient.reviewRepo != "wiki" || giteaClient.reviewPRNumber != 12 {
		t.Fatalf("unexpected review publish target owner=%s repo=%s pr=%d", giteaClient.reviewOwner, giteaClient.reviewRepo, giteaClient.reviewPRNumber)
	}
	if giteaClient.reviewOptions.Event != "COMMENT" || !strings.Contains(giteaClient.reviewOptions.Body, "Review final") {
		t.Fatalf("unexpected review options %#v", giteaClient.reviewOptions)
	}

	promptBase, err := os.ReadFile(filepath.Join(reviewLogDir, "02-prompt-base.log"))
	if err != nil {
		t.Fatalf("expected prompt base log file, got %v", err)
	}

	if !strings.Contains(string(promptBase), "PROMPT BASE REVIEW PARCIAL") {
		t.Fatalf("expected prompt base content, got %q", string(promptBase))
	}

	if !strings.Contains(string(promptBase), "PROMPT BASE REVIEW PARCIAL") ||
		!strings.Contains(string(promptBase), "PROMPT BASE REVIEW FINAL") {
		t.Fatalf("expected prompt base content, got %q", string(promptBase))
	}

	blockResponse, err := os.ReadFile(filepath.Join(reviewLogDir, "block-001-resposta.log"))
	if err != nil {
		t.Fatalf("expected block response log file, got %v", err)
	}

	blockResponseContent := string(blockResponse)
	if !strings.Contains(blockResponseContent, "tipo=bloco") ||
		!strings.Contains(blockResponseContent, "response_duration=") ||
		!strings.Contains(blockResponseContent, "total_desde_inicio_ollama=") ||
		!strings.Contains(blockResponseContent, "review bloco 1") {
		t.Fatalf("unexpected block response log %q", string(blockResponse))
	}

	finalResponse, err := os.ReadFile(filepath.Join(reviewLogDir, "final-resposta.log"))
	if err != nil {
		t.Fatalf("expected final response log file, got %v", err)
	}

	finalResponseContent := string(finalResponse)
	if !strings.Contains(finalResponseContent, "tipo=final") ||
		!strings.Contains(finalResponseContent, "response_duration=") ||
		!strings.Contains(finalResponseContent, "total_desde_inicio_ollama=") ||
		!strings.Contains(finalResponseContent, validFinalResponse()) {
		t.Fatalf("unexpected final response log %q", string(finalResponse))
	}
}

func TestReviewerAgentReturnsErrorWhenDiffFetchFails(t *testing.T) {
	giteaClient := &fakeGiteaClient{err: errors.New("gitea unavailable")}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), giteaClient, &fakeOllamaClient{}, ReviewerOptions{DiffLogDir: t.TempDir(), ReviewPromptConfigPath: testPromptConfigPath()})

	err := agent.Process(context.Background(), queue.ReviewJob{
		Owner:    "Qualyagro",
		Repo:     "wiki",
		PRNumber: 12,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegistryGetsRegisteredAgent(t *testing.T) {
	registry := NewRegistry()
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), &fakeGiteaClient{}, &fakeOllamaClient{}, ReviewerOptions{DiffLogDir: t.TempDir(), ReviewPromptConfigPath: testPromptConfigPath()})

	registry.Register(agent)

	got, err := registry.Get(ReviewerAgentName)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got != agent {
		t.Fatal("expected registered agent")
	}
}

func TestReviewerAgentReturnsErrorWhenOllamaFails(t *testing.T) {
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	agent := NewReviewerAgent(
		log.New(&bytes.Buffer{}, "", 0),
		giteaClient,
		&fakeOllamaClient{err: errors.New("ollama unavailable")},
		ReviewerOptions{DiffLogDir: t.TempDir(), MaxBlockChars: 4000, MaxFilesPerBlock: 2, ReviewPromptConfigPath: testPromptConfigPath()},
	)

	err := agent.Process(context.Background(), queue.ReviewJob{
		Owner:    "Qualyagro",
		Repo:     "wiki",
		PRNumber: 12,
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "todos os 1 blocos falharam") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestReviewerAgentContinuesWhenOneBlockFails(t *testing.T) {
	var logs bytes.Buffer
	diffLogDir := t.TempDir()
	ollamaClient := &fakeOllamaClient{
		model:      "deepseek-coder:6.7b",
		callErrors: []error{errors.New("timeout")},
		responses:  []string{"", "review bloco 2", validFinalResponse()},
	}
	agent := NewReviewerAgent(
		log.New(&logs, "", 0),
		&fakeGiteaClient{diff: sampleDiff()},
		ollamaClient,
		ReviewerOptions{DiffLogDir: diffLogDir, MaxBlockChars: 4000, MaxFilesPerBlock: 1, ReviewPromptConfigPath: testPromptConfigPath()},
	)

	err := agent.Process(context.Background(), queue.ReviewJob{
		Owner:    "Qualyagro",
		Repo:     "wiki",
		PRNumber: 12,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(ollamaClient.prompts) != 3 {
		t.Fatalf("expected 3 ollama calls, got %d", len(ollamaClient.prompts))
	}

	if !strings.Contains(ollamaClient.prompts[2], "analise e parcial") || !strings.Contains(ollamaClient.prompts[2], "STATUS: FALHA") {
		t.Fatalf("expected final prompt to mention partial analysis, got %q", ollamaClient.prompts[2])
	}

	if !strings.Contains(logs.String(), "erro ao revisar bloco com ollama block=1 total=2 files=1") {
		t.Fatalf("expected block error log, got %q", logs.String())
	}

	if !strings.Contains(logs.String(), "gerando review final partial_reviews=2 failed_blocks=1") {
		t.Fatalf("expected final generation with failed block log, got %q", logs.String())
	}

	reviewLogDir := filepath.Join(diffLogDir, "Qualyagro_wiki_pr-12")
	blockError, err := os.ReadFile(filepath.Join(reviewLogDir, "block-001-erro.log"))
	if err != nil {
		t.Fatalf("expected block error log file, got %v", err)
	}

	blockErrorContent := string(blockError)
	if !strings.Contains(blockErrorContent, "status=erro") ||
		!strings.Contains(blockErrorContent, "response_duration=") ||
		!strings.Contains(blockErrorContent, "total_desde_inicio_ollama=") ||
		!strings.Contains(blockErrorContent, "Falha ao revisar este bloco: timeout") {
		t.Fatalf("unexpected block error content %q", string(blockError))
	}

	finalResponse, err := os.ReadFile(filepath.Join(reviewLogDir, "final-resposta.log"))
	if err != nil {
		t.Fatalf("expected final response log, got %v", err)
	}

	finalResponseContent := string(finalResponse)
	if !strings.Contains(finalResponseContent, "failed_blocks=1") ||
		!strings.Contains(finalResponseContent, "total_desde_inicio_ollama=") ||
		!strings.Contains(finalResponseContent, validFinalResponse()) {
		t.Fatalf("unexpected final response %q", string(finalResponse))
	}
}

func TestReviewerAgentStopsWhenContextIsCanceled(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	ollamaClient := &fakeOllamaClient{
		callErrors: []error{context.Canceled},
	}
	ollamaClient.onCall = func(index int) {
		if index == 0 {
			cancel()
		}
	}
	agent := NewReviewerAgent(
		log.New(&logs, "", 0),
		&fakeGiteaClient{diff: sampleDiff()},
		ollamaClient,
		ReviewerOptions{DiffLogDir: t.TempDir(), MaxBlockChars: 4000, MaxFilesPerBlock: 1, ReviewPromptConfigPath: testPromptConfigPath()},
	)

	err := agent.Process(ctx, queue.ReviewJob{
		Owner:    "Qualyagro",
		Repo:     "wiki",
		PRNumber: 12,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}

	if !strings.Contains(err.Error(), "review cancelado durante bloco 1/2") {
		t.Fatalf("unexpected cancellation error %v", err)
	}

	if len(ollamaClient.prompts) != 1 {
		t.Fatalf("expected only first block prompt, got %d", len(ollamaClient.prompts))
	}

	if !strings.Contains(logs.String(), "review cancelado durante bloco block=1 total=2") {
		t.Fatalf("expected cancellation log, got %q", logs.String())
	}
}

func TestReviewerAgentRetriesInvalidFinalResponse(t *testing.T) {
	client := &fakeOllamaClient{
		responses: []string{"partial", "not json", validFinalResponse()},
	}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), &fakeGiteaClient{diff: sampleDiff()}, client, ReviewerOptions{
		DiffLogDir: t.TempDir(), MaxBlockChars: 4000, MaxFilesPerBlock: 2, ReviewPromptConfigPath: testPromptConfigPath(),
	})
	if err := agent.Process(context.Background(), queue.ReviewJob{Owner: "o", Repo: "r", PRNumber: 1}); err != nil {
		t.Fatalf("expected retry to recover: %v", err)
	}
	if len(client.prompts) != 3 {
		t.Fatalf("expected one partial call and two final calls, got %d", len(client.prompts))
	}
	if !strings.Contains(client.prompts[2], "CORRECAO OBRIGATORIA") || !strings.Contains(client.prompts[2], "JSON invalido") {
		t.Fatalf("expected specific correction prompt, got %q", client.prompts[2])
	}
}

func TestReviewerAgentStopsAfterThreeInvalidFinalResponses(t *testing.T) {
	client := &fakeOllamaClient{responses: []string{"partial", "x", "y", "z"}}
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), giteaClient, client, ReviewerOptions{
		DiffLogDir: t.TempDir(), MaxBlockChars: 4000, MaxFilesPerBlock: 2, ReviewPromptConfigPath: testPromptConfigPath(),
	})
	err := agent.Process(context.Background(), queue.ReviewJob{Owner: "o", Repo: "r", PRNumber: 1})
	if err == nil || !strings.Contains(err.Error(), "apos 3 tentativas") {
		t.Fatalf("expected final validation error, got %v", err)
	}
	if len(client.prompts) != 4 {
		t.Fatalf("expected one partial call plus three final calls, got %d", len(client.prompts))
	}
	if giteaClient.reviewCreated {
		t.Fatal("must not publish an invalid review")
	}
}

func TestBuildCreatePullReviewOptions(t *testing.T) {
	parsed := review.ParseFinalReviewResponse(`COMENTARIOS_INLINE:
- SEVERIDADE: alta
  PATH: app/file.go
  NEW_POSITION: 10
  TRECHO_REFERENCIA: broken()
  TITULO: Quebra
  BODY: Corrija esta chamada.
  MOTIVO_DECISAO: Bloqueante.
- SEVERIDADE: media
  PATH: app/other.go
  NEW_POSITION: 0
  TRECHO_REFERENCIA: status 200
  TITULO: Sem linha
  BODY: Revise este retorno.
  MOTIVO_DECISAO: Sem posicao.
REVISAO_FINAL:
  EVENTO_GITEA: REQUEST_CHANGES
  STATUS: reprovado
  RESUMO: Ajustes necessarios.
  OBSERVACOES: todos os blocos analisados`)

	options := buildCreatePullReviewOptions(parsed)

	if options.Event != "REQUEST_CHANGES" {
		t.Fatalf("unexpected event %q", options.Event)
	}
	if len(options.Comments) != 1 {
		t.Fatalf("expected one inline comment with position, got %#v", options.Comments)
	}
	if options.Comments[0].Path != "app/file.go" || options.Comments[0].NewPosition != 10 || options.Comments[0].Body != "Corrija esta chamada." {
		t.Fatalf("unexpected inline comment %#v", options.Comments[0])
	}
	if !strings.Contains(options.Body, "Ajustes necessarios.") ||
		!strings.Contains(options.Body, "Comentarios sem linha especifica:") ||
		!strings.Contains(options.Body, "app/other.go") {
		t.Fatalf("expected summary and comments without position in body, got %q", options.Body)
	}
}

func TestBuildCreatePullReviewOptionsDowngradesNonBlockingTypo(t *testing.T) {
	parsed := review.ParseFinalReviewResponse(`COMENTARIOS_INLINE:
- SEVERIDADE: baixa
  PATH: app/file.go
  NEW_POSITION: 10
  TRECHO_REFERENCIA: systemEsists
  TITULO: Typo no nome
  BODY: Ajuste a grafia para systemExists se este for o contrato esperado.
  MOTIVO_DECISAO: Comentario nao bloqueante de escrita/nome.
REVISAO_FINAL:
  EVENTO_GITEA: REQUEST_CHANGES
  STATUS: reprovado
  RESUMO: Ha apenas ajuste de escrita.
  OBSERVACOES: todos os blocos analisados`)

	options := buildCreatePullReviewOptions(parsed)

	if options.Event != "COMMENT" {
		t.Fatalf("expected non-blocking typo to become COMMENT, got %q", options.Event)
	}
	if len(options.Comments) != 1 {
		t.Fatalf("expected one comment, got %#v", options.Comments)
	}
}

type fakeGiteaClient struct {
	diff           string
	err            error
	reviewErr      error
	owner          string
	repo           string
	prNumber       int
	reviewCreated  bool
	reviewOwner    string
	reviewRepo     string
	reviewPRNumber int
	reviewOptions  gitea.CreatePullReviewOptions
}

type fakeOllamaClient struct {
	model      string
	responses  []string
	callErrors []error
	prompts    []string
	err        error
	onCall     func(index int)
}

func (c *fakeOllamaClient) Chat(ctx context.Context, prompt string) (string, error) {
	c.prompts = append(c.prompts, prompt)
	index := len(c.prompts) - 1
	if c.onCall != nil {
		c.onCall(index)
	}

	if c.err != nil {
		return "", c.err
	}

	if index < len(c.callErrors) && c.callErrors[index] != nil {
		return "", c.callErrors[index]
	}

	if index >= len(c.responses) {
		return "ok", nil
	}

	return c.responses[index], nil
}

func (c *fakeOllamaClient) Model() string {
	if c.model == "" {
		return "test-model"
	}

	return c.model
}

func (c *fakeGiteaClient) GetPullRequestDiff(ctx context.Context, owner string, repo string, prNumber int) (string, error) {
	c.owner = owner
	c.repo = repo
	c.prNumber = prNumber

	if c.err != nil {
		return "", c.err
	}

	return c.diff, nil
}

func (c *fakeGiteaClient) CreatePullRequestReview(ctx context.Context, owner string, repo string, prNumber int, options gitea.CreatePullReviewOptions) (gitea.PullReview, error) {
	c.reviewCreated = true
	c.reviewOwner = owner
	c.reviewRepo = repo
	c.reviewPRNumber = prNumber
	c.reviewOptions = options

	if c.reviewErr != nil {
		return gitea.PullReview{}, c.reviewErr
	}

	return gitea.PullReview{ID: 123, State: options.Event}, nil
}

func testPromptConfigPath() string {
	return filepath.Join("..", "..", "config", "review-prompts.yaml")
}

func sampleDiff() string {
	return `diff --git a/README.md b/README.md
index 111..222 100644
--- a/README.md
+++ b/README.md
@@ -1,2 +1,3 @@
 line one
-old line
+new line
+another line
diff --git a/internal/app.go b/internal/app.go
index 333..444 100644
--- a/internal/app.go
+++ b/internal/app.go
@@ -10,1 +10,1 @@
-fmt.Println("old")
+fmt.Println("new")
diff --git a/internal/service.go b/internal/service.go
index 555..666 100644
--- a/internal/service.go
+++ b/internal/service.go
@@ -20,1 +20,1 @@
-return oldValue
+return newValue
`
}

func validFinalResponse() string {
	return `{"comments":[],"final_review":{"gitea_event":"COMMENT","status":"comentario","summary":"Review final","observations":"Todos os blocos analisados."}}`
}
