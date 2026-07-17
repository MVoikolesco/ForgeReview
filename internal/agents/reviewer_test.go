package agents

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea-agents/internal/gitea"
	"gitea-agents/internal/queue"
	"gitea-agents/internal/review"
	"gitea-agents/internal/reviewconfig"
)

func TestReviewerClientFromCloudConfigSendsBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != "Bearer cloud-key" {
			t.Fatalf("path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	defer server.Close()
	client, err := reviewerClientFromConfig(&reviewconfig.ReviewConfig{
		Provider:   reviewconfig.ProviderConfig{Name: "ollama"},
		Connection: reviewconfig.ConnectionConfig{BaseURL: server.URL, APIKey: "cloud-key"},
		Model:      reviewconfig.ModelConfig{Name: "cloud-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Chat(context.Background(), "review"); err != nil {
		t.Fatal(err)
	}
}

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

	if len(ollamaClient.prompts) != 4 {
		t.Fatalf("expected 4 provider calls, got %d", len(ollamaClient.prompts))
	}

	if !strings.Contains(ollamaClient.prompts[0], "<etapa>planner</etapa>") {
		t.Fatalf("expected planner prompt, got %q", ollamaClient.prompts[0])
	}

	if !strings.Contains(ollamaClient.prompts[1], "<etapa>reviewer</etapa>") {
		t.Fatalf("expected reviewer prompt, got %q", ollamaClient.prompts[1])
	}

	if strings.Contains(ollamaClient.prompts[0], "README.md") || strings.Contains(ollamaClient.prompts[1], "README.md") {
		t.Fatalf("expected README to be ignored in pipeline prompts, got %q / %q", ollamaClient.prompts[0], ollamaClient.prompts[1])
	}

	if !strings.Contains(ollamaClient.prompts[2], "<etapa>consolidator</etapa>") || !strings.Contains(ollamaClient.prompts[3], "<etapa>formatter</etapa>") {
		t.Fatalf("expected consolidator and formatter prompts, got %#v", ollamaClient.prompts)
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

	if !strings.Contains(logs.String(), "pipeline iniciado") || !strings.Contains(logs.String(), "planner concluido") || !strings.Contains(logs.String(), "pipeline concluido") {
		t.Fatalf("expected pipeline logs, got %q", logs.String())
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

	finalResponse, err := os.ReadFile(filepath.Join(reviewLogDir, "final-resposta.log"))
	if err != nil {
		t.Fatalf("expected final response log file, got %v", err)
	}

	finalResponseContent := string(finalResponse)
	if !strings.Contains(finalResponseContent, "tipo=final") ||
		!strings.Contains(finalResponseContent, "response_duration=") ||
		!strings.Contains(finalResponseContent, "pipeline_version=2") {
		t.Fatalf("unexpected final response log %q", string(finalResponse))
	}
}

func TestReviewerAgentSavesManualReviewWithoutPublishing(t *testing.T) {
	diffLogDir := t.TempDir()
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	ollamaClient := &fakeOllamaClient{
		model:     "deepseek-coder:6.7b",
		responses: []string{"review bloco 1", "review bloco 2", validFinalResponse()},
	}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), giteaClient, ollamaClient, ReviewerOptions{
		DiffLogDir:             diffLogDir,
		MaxBlockChars:          4000,
		MaxFilesPerBlock:       1,
		ReviewPromptConfigPath: testPromptConfigPath(),
	})

	job := queue.ReviewJob{Owner: "Qualyagro", Repo: "wiki", PRNumber: 12, Manual: true}
	if err := agent.Process(context.Background(), job); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if giteaClient.reviewCreated {
		t.Fatal("expected manual review not to be published to gitea")
	}

	finalReview, err := os.ReadFile(filepath.Join(diffLogDir, "Qualyagro_wiki_pr-12", "final-review.md"))
	if err != nil {
		t.Fatalf("expected final markdown file, got %v", err)
	}
	if !strings.Contains(string(finalReview), "# Review final") {
		t.Fatalf("expected markdown heading, got %q", finalReview)
	}
}

func TestReviewerAgentPublishesManualReviewWhenEnabled(t *testing.T) {
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	ollamaClient := &fakeOllamaClient{
		model:     "deepseek-coder:6.7b",
		responses: []string{"review bloco 1", "review bloco 2", validFinalResponse()},
	}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), giteaClient, ollamaClient, ReviewerOptions{
		DiffLogDir:             t.TempDir(),
		MaxBlockChars:          4000,
		MaxFilesPerBlock:       1,
		ReviewPromptConfigPath: testPromptConfigPath(),
		PublishManualReviews:   true,
	})

	job := queue.ReviewJob{Owner: "Qualyagro", Repo: "wiki", PRNumber: 12, Manual: true}
	if err := agent.Process(context.Background(), job); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !giteaClient.reviewCreated {
		t.Fatal("expected manual review to be published to gitea")
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

	if !strings.Contains(err.Error(), "todos os grupos falharam") {
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

	if len(ollamaClient.prompts) < 4 {
		t.Fatalf("expected pipeline calls, got %d", len(ollamaClient.prompts))
	}
	reviewLogDir := filepath.Join(diffLogDir, "Qualyagro_wiki_pr-12")
	finalResponse, err := os.ReadFile(filepath.Join(reviewLogDir, "final-resposta.log"))
	if err != nil {
		t.Fatalf("expected final response log, got %v", err)
	}

	finalResponseContent := string(finalResponse)
	if !strings.Contains(finalResponseContent, "pipeline_version=2") {
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

	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("unexpected cancellation error %v", err)
	}

	if len(ollamaClient.prompts) != 1 {
		t.Fatalf("expected only first block prompt, got %d", len(ollamaClient.prompts))
	}

	_ = logs
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
	if len(client.prompts) < 4 {
		t.Fatalf("expected pipeline calls, got %d", len(client.prompts))
	}
}

func TestReviewerAgentStopsAfterThreeInvalidFinalResponses(t *testing.T) {
	client := &fakeOllamaClient{responses: []string{"partial", "x", "y", "z"}}
	giteaClient := &fakeGiteaClient{diff: sampleDiff()}
	agent := NewReviewerAgent(log.New(&bytes.Buffer{}, "", 0), giteaClient, client, ReviewerOptions{
		DiffLogDir: t.TempDir(), MaxBlockChars: 4000, MaxFilesPerBlock: 2, ReviewFinalRetries: 3, ReviewPromptConfigPath: testPromptConfigPath(),
	})
	err := agent.Process(context.Background(), queue.ReviewJob{Owner: "o", Repo: "r", PRNumber: 1})
	if err != nil {
		t.Fatalf("expected formatter fallback to publish deterministic review, got %v", err)
	}
	if len(client.prompts) < 4 {
		t.Fatalf("expected pipeline calls, got %d", len(client.prompts))
	}
	if !giteaClient.reviewCreated {
		t.Fatal("expected deterministic fallback review to be published")
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

	options := buildCreatePullReviewOptions(parsed, true)

	if options.Event != "REQUEST_CHANGES" {
		t.Fatalf("unexpected event %q", options.Event)
	}
	if len(options.Comments) != 1 {
		t.Fatalf("expected one inline comment with position, got %#v", options.Comments)
	}
	if options.Comments[0].Path != "app/file.go" || options.Comments[0].NewPosition != 10 || !strings.Contains(options.Comments[0].Body, "> severity: alta") || !strings.Contains(options.Comments[0].Body, "Bloqueante.") || !strings.Contains(options.Comments[0].Body, "Corrija esta chamada.") {
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

	options := buildCreatePullReviewOptions(parsed, true)

	if options.Event != "COMMENT" {
		t.Fatalf("expected non-blocking typo to become COMMENT, got %q", options.Event)
	}
	if len(options.Comments) != 1 {
		t.Fatalf("expected one comment, got %#v", options.Comments)
	}
}

func TestBuildCreatePullReviewOptionsDoesNotRejectWhenAutonomyIsDisabled(t *testing.T) {
	parsed := review.ParseFinalReviewResponse(`REVISAO_FINAL:
  EVENTO_GITEA: REQUEST_CHANGES
  STATUS: reprovado
  RESUMO: Ajustes necessarios.
  OBSERVACOES: Revisao concluida.`)

	options := buildCreatePullReviewOptions(parsed, false)

	if options.Event != review.GiteaEventComment {
		t.Fatalf("expected rejection to become COMMENT, got %q", options.Event)
	}
	if !strings.Contains(options.Body, "> status: reprovado") {
		t.Fatalf("expected status in review body, got %q", options.Body)
	}
}

func TestBuildCreatePullReviewOptionsDoesNotApproveWhenAutonomyIsDisabled(t *testing.T) {
	parsed := review.ParseFinalReviewResponse(`COMENTARIOS_INLINE:
- SEVERIDADE: baixa
  PATH: app/file.go
  NEW_POSITION: 10
  BODY: Boa implementacao.
REVISAO_FINAL:
  EVENTO_GITEA: APPROVED
  STATUS: aprovado
  RESUMO: Revisao aprovada.
  OBSERVACOES: Revisao concluida.`)

	options := buildCreatePullReviewOptions(parsed, false)

	if options.Event != review.GiteaEventComment {
		t.Fatalf("expected approval to become COMMENT, got %q", options.Event)
	}
	if !strings.Contains(options.Body, "> status: aprovado") {
		t.Fatalf("expected status in review body, got %q", options.Body)
	}
	if len(options.Comments) != 1 || options.Comments[0].Path != "app/file.go" {
		t.Fatalf("expected inline comment to be preserved, got %#v", options.Comments)
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

	if stage := fakePipelineResponse(prompt); stage != "" {
		return stage, nil
	}

	if index >= len(c.responses) {
		return validFinalResponse(), nil
	}

	return c.responses[index], nil
}

func fakePipelineResponse(prompt string) string {
	switch {
	case strings.Contains(prompt, "<etapa>planner</etapa>"):
		return `{"pr_summary":"Resumo do PR","risk_level":"medio","risk_areas":["contratos"],"groups":[{"id":"group-1","purpose":"Arquivos Go","files":["internal/app.go","internal/service.go"],"risk_level":"medio","review_focus":["contratos"]}],"assumptions":[]}`
	case strings.Contains(prompt, "<etapa>reviewer</etapa>"):
		return `{"group_id":"group-1","reviewed_files":["internal/app.go","internal/service.go"],"findings":[],"review_summary":"Sem achados."}`
	case strings.Contains(prompt, "<etapa>consolidator</etapa>"):
		return `{"pr_summary":"Review final","overall_risk":"baixo","findings":[],"discarded_findings":[]}`
	case strings.Contains(prompt, "<etapa>verifier</etapa>"):
		return `{"results":[]}`
	case strings.Contains(prompt, "<etapa>formatter</etapa>"):
		return validFinalResponse()
	}
	return ""
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
