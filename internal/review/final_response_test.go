package review

import (
	"strings"
	"testing"

	"gitea-agents/internal/diff"
)

func TestValidateFinalReviewResponse(t *testing.T) {
	valid := `{"comments":[{"file":"app/file.go","line":3,"severity":"media","decision_reason":"Regressao","comment":"Corrija o contrato."}],"final_review":{"gitea_event":"REQUEST_CHANGES","status":"reprovado","summary":"Ha um problema.","observations":"Bloco analisado."}}`
	parsed, err := ValidateFinalReviewResponse(valid)
	if err != nil || len(parsed.InlineComments) != 1 || parsed.InlineComments[0].NewPosition != 3 {
		t.Fatalf("expected valid response, got %#v, %v", parsed, err)
	}
}

func TestValidateFinalReviewResponseRejectsInvalidContracts(t *testing.T) {
	base := `{"comments":[],"final_review":{"gitea_event":"COMMENT","status":"comentario","summary":"ok","observations":""}}`
	cases := []struct {
		name string
		body string
		want string
	}{
		{"invalid json", "{", "JSON invalido"},
		{"text around json", "antes " + base, "JSON invalido"},
		{"markdown", "```json\n" + base + "\n```", "JSON invalido"},
		{"severity", strings.Replace(base, `"comments":[]`, `"comments":[{"file":"a","line":1,"severity":"critica","decision_reason":"x","comment":"x"}]`, 1), "severity nao permitida"},
		{"status", strings.Replace(base, `"status":"comentario"`, `"status":"invalido"`, 1), "status nao permitido"},
		{"event", strings.Replace(base, `"gitea_event":"COMMENT"`, `"gitea_event":"MERGE"`, 1), "gitea_event nao permitido"},
		{"line type", strings.Replace(base, `"comments":[]`, `"comments":[{"file":"a","line":"1","severity":"baixa","decision_reason":"x","comment":"x"}]`, 1), "estrutura JSON invalida"},
		{"missing field", strings.Replace(base, `,"observations":""`, "", 1), "final_review.observations ausente"},
		{"unknown field", strings.Replace(base, `"comments":[]`, `"comments":[],"extra":true`, 1), "propriedades de primeiro nivel"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateFinalReviewResponse(test.body)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateFinalReviewResponseAcceptsEmptyComments(t *testing.T) {
	_, err := ValidateFinalReviewResponse(`{"comments":[],"final_review":{"gitea_event":"APPROVED","status":"aprovado","summary":"Sem problemas.","observations":""}}`)
	if err != nil {
		t.Fatalf("expected empty comments to be valid: %v", err)
	}
}

func TestParseFinalReviewResponse(t *testing.T) {
	content := `COMENTARIOS_INLINE:
- SEVERIDADE: alta
  PATH: app/file.go
  NEW_POSITION: 42
  TRECHO_REFERENCIA: risky()
  TITULO: Chamada insegura
  BODY: Ajuste esta chamada antes do merge.
  MOTIVO_DECISAO: Quebra o fluxo.
- SEVERIDADE: media
  PATH: app/other.go
  NEW_POSITION: 0
  TRECHO_REFERENCIA: status 200
  TITULO: Status incorreto
  BODY: Use codigo de erro no corpo da resposta.
  MOTIVO_DECISAO: Contrato inconsistente.

REVISAO_FINAL:
  EVENTO_GITEA: REQUEST_CHANGES
  STATUS: reprovado
  RESUMO: Ha problemas bloqueantes.
  OBSERVACOES: todos os blocos analisados`

	parsed := ParseFinalReviewResponse(content)

	if !parsed.Structured {
		t.Fatal("expected structured response")
	}
	if parsed.Event != GiteaEventRequestChanges {
		t.Fatalf("expected request changes, got %q", parsed.Event)
	}
	if parsed.Summary != "Ha problemas bloqueantes." {
		t.Fatalf("unexpected summary %q", parsed.Summary)
	}
	if len(parsed.InlineComments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(parsed.InlineComments))
	}
	if parsed.InlineComments[0].Path != "app/file.go" || parsed.InlineComments[0].NewPosition != 42 {
		t.Fatalf("unexpected first comment %#v", parsed.InlineComments[0])
	}
	if withoutPosition := parsed.CommentsWithoutPosition(); len(withoutPosition) != 1 || withoutPosition[0].Path != "app/other.go" {
		t.Fatalf("expected one comment without position, got %#v", withoutPosition)
	}
}

func TestParseFinalReviewResponseFallsBackForPlainText(t *testing.T) {
	parsed := ParseFinalReviewResponse("review final simples")

	if parsed.Structured {
		t.Fatal("expected unstructured response")
	}
	if parsed.Event != GiteaEventComment {
		t.Fatalf("expected comment event, got %q", parsed.Event)
	}
	if parsed.ReviewBody() != "review final simples" {
		t.Fatalf("unexpected body %q", parsed.ReviewBody())
	}
}

func TestReviewBodyIncludesMetadataHeader(t *testing.T) {
	review := FinalReview{
		Status:  "aprovado",
		Summary: "Resumo do review.",
		Metadata: ReviewMetadata{
			Model:            "gemma4:31b-cloud",
			Elapsed:          "14.708s",
			PromptTokens:     123,
			CompletionTokens: 45,
		},
	}

	expected := "> status: aprovado\n> elapsed time: 14.708s\n> model: gemma4:31b-cloud\n> tokens: 168 (prompt: 123, completion: 45)\n\nResumo do review."
	if got := review.ReviewBody(); got != expected {
		t.Fatalf("unexpected body %q", got)
	}
}

func TestResolveFinalReviewCommentPositions(t *testing.T) {
	files := diff.Parse(`diff --git a/app/Controllers/Oracle/OracleAccess.php b/app/Controllers/Oracle/OracleAccess.php
index 7bc9fb4..bf1b437 100644
--- a/app/Controllers/Oracle/OracleAccess.php
+++ b/app/Controllers/Oracle/OracleAccess.php
@@ -23,9 +23,9 @@ class OracleAccess extends BaseController
     {
         $json = $this->request->getJSON();
 
-        if (empty($json->id_usuario) || empty($json->email) || empty($json->serial)) {
+        if (empty($json->id_usuario) || empty($json->email)) {
             return $this->response->setJSON([
-                'status' => 400,
+                'status' => 200,
                 'body' => 'Campos obrigatorios faltando: id_usuario, email e serial'
             ]);
         }
@@ -39,22 +39,22 @@ class OracleAccess extends BaseController
         }
 
         // Valida se o sistema existe
-        if (!$this->oracleAccessModel->systemExists($json->serial)) {
+        if (!empty($json->serial) && !$this->oracleAccessModel->systemEsists($json->serial)) {
             return $this->response->setJSON([
                 'status' => 404,
                'body' => 'Sistema nao encontrado com o serial fornecido'
             ]);
         }
 
-        $token     = bin2hex(random_bytes(32));
+        $token     = md5($json->email . time());
`)
	finalReview := FinalReview{
		InlineComments: []InlineComment{
			{Path: "app/Controllers/Oracle/OracleAccess.php", Reference: "$token = md5($json->email . time());", NewPosition: 45},
			{Path: "app/Controllers/Oracle/OracleAccess.php", Reference: "!$this->oracleAccessModel->systemEsists($json->serial)", NewPosition: 41},
			{Path: "app/Controllers/Oracle/OracleAccess.php", Reference: "'status' => 200,", NewPosition: 27},
			{Path: "app/Controllers/Oracle/OracleAccess.php", Reference: "referencia inexistente", NewPosition: 99},
		},
	}

	resolved := ResolveFinalReviewCommentPositions(finalReview, files)

	expected := []int{49, 42, 28, 0}
	for index, line := range expected {
		if resolved.InlineComments[index].NewPosition != line {
			t.Fatalf("comment %d expected line %d, got %d", index, line, resolved.InlineComments[index].NewPosition)
		}
	}
}
