package workflow

type Catalog struct{ cards map[string]CardType }

func NewCatalog(cards ...CardType) Catalog {
	items := make(map[string]CardType, len(cards))
	for _, card := range cards {
		items[card.Key] = card
	}
	return Catalog{cards: items}
}

func (c Catalog) Get(key string) (CardType, bool) { card, ok := c.cards[key]; return card, ok }
func (c Catalog) All() []CardType {
	items := make([]CardType, 0, len(c.cards))
	for _, card := range c.cards {
		items = append(items, card)
	}
	return items
}

func DefaultCatalog() Catalog {
	in := func(key, label, contract string, required bool) Port {
		return Port{Key: key, Label: label, Contract: contract, Required: required}
	}
	out := func(key, label, contract string) Port { return Port{Key: key, Label: label, Contract: contract} }
	card := func(key, name, category, description string, inputs, outputs []Port) CardType {
		return CardType{Key: key, Name: name, Category: category, Description: description, Inputs: inputs, Outputs: outputs}
	}
	return NewCatalog(
		card("trigger", "Trigger", "Entradas", "Inicia uma execução por evento, API, agenda ou ação manual.", nil, []Port{out("event", "Evento", "event")}),
		card("fetch", "Buscar dados", "Dados", "Coleta dados do pull request por uma integração selecionada.", []Port{in("event", "Evento", "event", true)}, []Port{out("pull_request", "Dados do PR", "pull_request"), out("files", "Arquivos", "files")}),
		card("filter", "Filtro", "Transformação", "Inclui ou exclui arquivos por regras configuradas.", []Port{in("files", "Arquivos", "files", true)}, []Port{out("files", "Arquivos filtrados", "files")}),
		card("group", "Agrupar", "Transformação", "Cria grupos de arquivos para processamento.", []Port{in("files", "Arquivos", "files", true)}, []Port{out("groups", "Grupos", "groups")}),
		card("transform", "Transformar", "Transformação", "Normaliza, limita ou reorganiza dados tipados.", []Port{in("input", "Entrada", "any", true)}, []Port{out("output", "Saída", "any")}),
		card("template", "Template", "Transformação", "Monta um prompt a partir de variáveis e contexto.", []Port{in("context", "Contexto", "any", true)}, []Port{out("prompt", "Prompt", "prompt")}),
		card("variable", "Variáveis", "Transformação", "Declara ou atualiza variáveis do escopo da execução.", []Port{in("value", "Valor", "any", false)}, []Port{out("value", "Valor", "any")}),
		card("condition", "Condição", "Controle", "Roteia dados para saídas nomeadas conforme regras.", []Port{in("input", "Entrada", "any", true)}, []Port{out("true", "Verdadeiro", "any"), out("false", "Falso", "any")}),
		card("loop", "Loop", "Controle", "Itera uma lista com escopo, concorrência e limite configurados.", []Port{in("items", "Itens", "list", true)}, []Port{out("item", "Item atual", "any"), out("results", "Resultados", "list")}),
		card("merge", "Merge", "Controle", "Aguarda e combina resultados de ramos.", []Port{in("inputs", "Entradas", "any", true)}, []Port{out("output", "Resultado unido", "any")}),
		card("workflow", "Workflow", "Controle", "Executa uma subpipeline publicada por sua interface declarada.", []Port{in("input", "Entrada", "any", false)}, []Port{out("output", "Saída", "any")}),
		card("model", "Modelo IA", "IA", "Executa um modelo por um adaptador de provider.", []Port{in("prompt", "Prompt", "prompt", true)}, []Port{out("response", "Resposta", "model_response")}),
		card("validate", "Validar", "Validação", "Valida schema, semântica e referências da resposta.", []Port{in("response", "Resposta", "model_response", true)}, []Port{out("valid", "Resposta válida", "validated_response"), out("invalid", "Resposta inválida", "error")}),
		card("response_filter", "Filtrar resposta", "Validação", "Remove resultados inválidos, duplicados ou abaixo da política.", []Port{in("response", "Resposta validada", "validated_response", true)}, []Port{out("comments", "Comentários", "comments")}),
		card("consolidate", "Consolidar", "Resultado", "Consolida comentários e produz uma revisão.", []Port{in("comments", "Comentários", "comments", true)}, []Port{out("review", "Review consolidada", "review")}),
		card("format", "Formatar", "Resultado", "Formata uma revisão para o destino configurado.", []Port{in("review", "Review", "review", true)}, []Port{out("formatted", "Resultado formatado", "formatted_review")}),
		card("publish", "Publicar", "Saída", "Publica um resultado por uma integração cadastrada.", []Port{in("result", "Resultado", "formatted_review", true)}, []Port{out("receipt", "Comprovante", "publication")}),
		card("log", "Log", "Infraestrutura", "Registra dados sanitizados para observabilidade.", []Port{in("input", "Entrada", "any", false)}, []Port{out("output", "Saída", "any")}),
		card("cache", "Cache", "Infraestrutura", "Lê ou grava dados efêmeros por uma chave configurada.", []Port{in("value", "Valor", "any", false)}, []Port{out("value", "Valor", "any")}),
		card("error_control", "Controle de erro", "Infraestrutura", "Aplica política avançada de retry, fallback ou encerramento.", []Port{in("error", "Erro", "error", true)}, []Port{out("recovered", "Recuperado", "any"), out("failed", "Falha", "error")}),
	)
}
