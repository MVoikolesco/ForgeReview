package review

import (
	"strings"
	"testing"

	"gitea-agents/internal/diff"
)

func TestBuildReviewBlocksRespectsMaxFilesPerBlock(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "a.go", Patch: patch("a.go", 20)},
		{Path: "b.go", Patch: patch("b.go", 20)},
		{Path: "c.go", Patch: patch("c.go", 20)},
	}

	blocks := BuildReviewBlocks(files, 10000, 2)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	if blocks[0].Index != 1 || blocks[0].Total != 2 || len(blocks[0].Files) != 2 {
		t.Fatalf("unexpected first block %#v", blocks[0])
	}

	if blocks[1].Index != 2 || blocks[1].Total != 2 || len(blocks[1].Files) != 1 {
		t.Fatalf("unexpected second block %#v", blocks[1])
	}
}

func TestBuildReviewBlocksUsesLightweightDefaults(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "a.go", Patch: patch("a.go", 20)},
		{Path: "b.go", Patch: patch("b.go", 20)},
		{Path: "c.go", Patch: patch("c.go", 20)},
	}

	blocks := BuildReviewBlocks(files, 0, 0)

	if len(blocks) != 2 {
		t.Fatalf("expected default max 2 files per block, got %d blocks", len(blocks))
	}
}

func TestBuildReviewBlocksRespectsMaxCharsWhenPossible(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "a.go", Patch: patch("a.go", 50)},
		{Path: "b.go", Patch: patch("b.go", 50)},
	}

	blocks := BuildReviewBlocks(files, len(formatReviewFile(files[0]))+10, 3)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestBuildReviewBlocksKeepsLargeFileAlone(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "large.go", Patch: patch("large.go", 500)},
		{Path: "small.go", Patch: patch("small.go", 20)},
	}

	blocks := BuildReviewBlocks(files, 100, 3)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	if len(blocks[0].Files) != 1 || blocks[0].Files[0] != "large.go" {
		t.Fatalf("expected large file alone, got %#v", blocks[0].Files)
	}
}

func TestBuildReviewBlocksIgnoresGeneratedFiles(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "package-lock.json", Patch: patch("package-lock.json", 10)},
		{Path: "frontend/package-lock.json", Patch: patch("frontend/package-lock.json", 10)},
		{Path: "go.sum", Patch: patch("go.sum", 10)},
		{Path: "backend/go.sum", Patch: patch("backend/go.sum", 10)},
		{Path: "vendor/lib.go", Patch: patch("vendor/lib.go", 10)},
		{Path: "node_modules/pkg/index.js", Patch: patch("node_modules/pkg/index.js", 10)},
		{Path: "dist/app.js", Patch: patch("dist/app.js", 10)},
		{Path: "build/app.js", Patch: patch("build/app.js", 10)},
		{Path: "src/app.js.map", Patch: patch("src/app.js.map", 10)},
		{Path: "src/app.min.js", Patch: patch("src/app.min.js", 10)},
		{Path: "README.md", Patch: patch("README.md", 10)},
		{Path: "docs/README.md", Patch: patch("docs/README.md", 10)},
		{Path: "docs/readme.pt-BR.md", Patch: patch("docs/readme.pt-BR.md", 10)},
		{Path: "docs/architecture.go", Patch: patch("docs/architecture.go", 10)},
		{Path: "CHANGELOG.md", Patch: patch("CHANGELOG.md", 10)},
		{Path: "CONTRIBUTING.md", Patch: patch("CONTRIBUTING.md", 10)},
		{Path: "guide.mdx", Patch: patch("guide.mdx", 10)},
		{Path: "config/review-prompts.yaml", Patch: patch("config/review-prompts.yaml", 10)},
		{Path: ".gitea/workflows/deploy.yml", Patch: patch(".gitea/workflows/deploy.yml", 10)},
		{Path: "docker-compose.yml", Patch: patch("docker-compose.yml", 10)},
		{Path: "LICENSE", Patch: patch("LICENSE", 10)},
		{Path: "src/app.go", Patch: patch("src/app.go", 10)},
	}

	blocks := BuildReviewBlocks(files, 10000, 3)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	if len(blocks[0].Files) != 1 || blocks[0].Files[0] != "src/app.go" {
		t.Fatalf("unexpected files %#v", blocks[0].Files)
	}
}

func TestBuildReviewBlocksIncludesDocsWhenConfigured(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "README.md", Patch: patch("README.md", 10)},
		{Path: ".env.example", Patch: patch(".env.example", 10)},
		{Path: ".gitignore", Patch: patch(".gitignore", 10)},
		{Path: "Dockerfile", Patch: patch("Dockerfile", 10)},
		{Path: ".gitea/workflows/deploy.yml", Patch: patch(".gitea/workflows/deploy.yml", 10)},
		{Path: "config/review-prompts.yaml", Patch: patch("config/review-prompts.yaml", 10)},
	}

	blocks := BuildReviewBlocksWithOptions(files, 10000, 10, true)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	got := strings.Join(blocks[0].Files, ",")
	for _, expected := range []string{"README.md", ".env.example", ".gitignore", "Dockerfile"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %s when docs are included, got %#v", expected, blocks[0].Files)
		}
	}
	for _, ignored := range []string{".gitea/workflows/deploy.yml", "config/review-prompts.yaml"} {
		if strings.Contains(got, ignored) {
			t.Fatalf("expected yaml file %s to be ignored, got %#v", ignored, blocks[0].Files)
		}
	}
}

func TestBuildReviewBlocksKeepsConfigFilesWhenDocsAreIgnored(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "README.md", Patch: patch("README.md", 10)},
		{Path: ".env.example", Patch: patch(".env.example", 10)},
		{Path: ".gitignore", Patch: patch(".gitignore", 10)},
		{Path: "Dockerfile", Patch: patch("Dockerfile", 10)},
		{Path: ".gitea/workflows/deploy.yml", Patch: patch(".gitea/workflows/deploy.yml", 10)},
		{Path: "docker-compose.yaml", Patch: patch("docker-compose.yaml", 10)},
	}

	blocks := BuildReviewBlocksWithOptions(files, 10000, 10, false)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	got := strings.Join(blocks[0].Files, ",")
	if strings.Contains(got, "README.md") {
		t.Fatalf("expected README to be ignored, got %#v", blocks[0].Files)
	}
	for _, expected := range []string{".env.example", ".gitignore", "Dockerfile"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %s to stay reviewable, got %#v", expected, blocks[0].Files)
		}
	}
	for _, ignored := range []string{".gitea/workflows/deploy.yml", "docker-compose.yaml"} {
		if strings.Contains(got, ignored) {
			t.Fatalf("expected yaml file %s to be ignored, got %#v", ignored, blocks[0].Files)
		}
	}
}

func TestBuildReviewBlocksKeepsFileDelimiters(t *testing.T) {
	files := []diff.ChangedFile{
		{Path: "src/app.go", Patch: patch("src/app.go", 10)},
	}

	blocks := BuildReviewBlocks(files, 10000, 3)

	if !strings.Contains(blocks[0].Content, "===== INICIO ARQUIVO path=src/app.go =====") {
		t.Fatalf("expected start delimiter, got %q", blocks[0].Content)
	}

	if !strings.Contains(blocks[0].Content, "===== FIM ARQUIVO path=src/app.go =====") {
		t.Fatalf("expected end delimiter, got %q", blocks[0].Content)
	}
}

func TestPromptsIncludeExpectedSections(t *testing.T) {
	block := ReviewBlock{Index: 1, Total: 1, Files: []string{"src/app.go"}, Content: "diff content"}

	partialPrompt := BuildPartialReviewPrompt(block, "prompt base resolvido\n\n## Achados\nImpacto: consequencia tecnica concreta")
	if !strings.Contains(partialPrompt, "Este e o bloco 1 de 1") ||
		!strings.Contains(partialPrompt, "## Bloco 1/1") ||
		!strings.Contains(partialPrompt, "## Achados") ||
		!strings.Contains(partialPrompt, "Impacto: consequencia tecnica concreta") ||
		!strings.Contains(partialPrompt, "```diff") ||
		!strings.Contains(partialPrompt, "ACHADOS_CONCRETOS:") ||
		!strings.Contains(partialPrompt, "CONTRATOS_DECLARADOS:") {
		t.Fatalf("unexpected partial prompt %q", partialPrompt)
	}

	previous := []PartialReview{{
		Block: ReviewBlock{Index: 1, Total: 2, Files: []string{"src/service.go"}},
		Content: `STATUS: aprovado
BLOCO: 1/2
RESUMO: bloco com contratos.
ACHADOS_CONCRETOS:
- nenhum
CONTRATOS_DECLARADOS:
- ARQUIVO: src/service.go
  TIPO: metodo
  NOME: UserService.teteMethod
REFERENCIAS_A_VERIFICAR:
- nenhum
REGRAS_DE_VALIDACAO:
- CAMPO: email
  REGRA: obrigatorio`,
	}}

	memoryPrompt := BuildPartialReviewPromptWithMemory(
		ReviewBlock{Index: 2, Total: 2, Files: []string{"src/handler.go"}, Content: "diff content"},
		"prompt base resolvido",
		previous,
	)
	if !strings.Contains(memoryPrompt, "Memoria tecnica acumulada") ||
		!strings.Contains(memoryPrompt, "UserService.teteMethod") ||
		!strings.Contains(memoryPrompt, "CAMPO: email") {
		t.Fatalf("expected accumulated memory in partial prompt, got %q", memoryPrompt)
	}

	finalPrompt := BuildFinalReviewPrompt(previous, "prompt final resolvido\n\n## Veredito")
	if !strings.Contains(finalPrompt, "Reviews parciais:") ||
		!strings.Contains(finalPrompt, "Quantidade de blocos: 1") ||
		!strings.Contains(finalPrompt, "## Veredito") ||
		!strings.Contains(finalPrompt, "Memoria tecnica consolidada para cruzamento") ||
		!strings.Contains(finalPrompt, "UserService.teteMethod") ||
		!strings.Contains(finalPrompt, "COMENTARIOS_INLINE:") ||
		!strings.Contains(finalPrompt, "EVENTO_GITEA: APPROVED|REQUEST_CHANGES|COMMENT") {
		t.Fatalf("unexpected final prompt %q", finalPrompt)
	}

	failedPrompt := BuildFinalReviewPrompt([]PartialReview{{Block: block, Content: "Falha ao revisar este bloco: timeout", Failed: true, Error: "timeout"}}, "prompt final resolvido")
	if !strings.Contains(failedPrompt, "analise e parcial") || !strings.Contains(failedPrompt, "STATUS: FALHA") {
		t.Fatalf("expected failed block context, got %q", failedPrompt)
	}
}

func TestPromptsIncludeAutomaticRegressionSignals(t *testing.T) {
	block := ReviewBlock{
		Index: 1,
		Total: 1,
		Files: []string{"app/Controllers/Oracle/OracleAccess.php"},
		Content: `diff --git a/app/Controllers/Oracle/OracleAccess.php b/app/Controllers/Oracle/OracleAccess.php
@@ -1,8 +1,8 @@
-if (empty($json->id_usuario) || empty($json->email) || empty($json->serial)) {
+if (empty($json->id_usuario) || empty($json->email)) {
-$token = bin2hex(random_bytes(32));
+$token = md5($json->email . time());
-if (!$this->oracleAccessModel->systemExists($json->serial)) {
+if (!empty($json->serial) && !$this->oracleAccessModel->systemEsists($json->serial)) {
-$expiresAt = date('Y-m-d H:i:s', strtotime('+1 hour'));
-$this->oracleAccessModel->insertToken($json->id_usuario, $json->email, $token, $json->serial, $expiresAt);
-    $this->oracleAccessModel->invalidateToken($json->token);
+    'status' => 200,
`,
	}

	partialPrompt := BuildPartialReviewPrompt(block, "prompt base")
	for _, expected := range []string{
		"Sinais automaticos de regressao",
		"validacao/permissao/sanitizacao removida",
		"geracao previsivel adicionada",
		"invalidacao ou cleanup removido",
		"contrato/chamada removido",
		"contrato/chamada adicionado",
		"codigo/status de sucesso em resposta",
	} {
		if !strings.Contains(partialPrompt, expected) {
			t.Fatalf("expected partial prompt to contain %q, got %q", expected, partialPrompt)
		}
	}
	signals := buildRegressionSignals(block.Content)
	for _, unexpected := range []string{
		"$expiresAt = date('Y-m-d H:i:s', strtotime('+1 hour'));",
		"seguranca ou segredo removido: `$this->oracleAccessModel->insertToken",
		"invalidacao ou cleanup removido: `$this->oracleAccessModel->insertToken",
	} {
		if strings.Contains(signals, unexpected) {
			t.Fatalf("expected regression signals not to contain noisy signal %q, got %q", unexpected, signals)
		}
	}

	finalPrompt := BuildFinalReviewPrompt([]PartialReview{{Block: block, Content: "STATUS: reprovado"}}, "prompt final")
	if !strings.Contains(finalPrompt, "Sinais automaticos de regressao consolidados") ||
		!strings.Contains(finalPrompt, "systemEsists") ||
		!strings.Contains(finalPrompt, "invalidateToken") {
		t.Fatalf("expected consolidated regression signals, got %q", finalPrompt)
	}
}

func patch(path string, size int) string {
	return "diff --git a/" + path + " b/" + path + "\n+" + strings.Repeat("x", size) + "\n"
}
