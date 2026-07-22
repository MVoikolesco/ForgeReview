package workflow

import "sort"

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
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items
}

func DefaultCatalog() Catalog {
	in := func(key, label, contract string, required bool) Port {
		return Port{Key: key, Label: label, Contract: contract, Required: required}
	}
	out := func(key, label, contract string) Port { return Port{Key: key, Label: label, Contract: contract} }
	card := func(key, name, category, description string, inputs, outputs []Port) CardType {
		return CardType{Key: key, Name: name, Category: category, Description: description, Inputs: inputs, Outputs: outputs, ErrorOutput: &Port{Key: "error", Label: "Erro", Contract: "error"}}
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
		card("loop", "Loop", "Controle", "Itera uma lista ou grupos com escopo, concorrência e limite configurados.", []Port{in("items", "Itens", "any", true)}, []Port{out("item", "Item atual", "any"), out("results", "Resultados", "list")}),
		card("merge", "Merge", "Controle", "Aguarda e combina resultados de ramos.", []Port{in("inputs", "Entradas", "any", true)}, []Port{out("output", "Resultado unido", "any")}),
		card("workflow", "Workflow", "Controle", "Executa uma subpipeline publicada por sua interface declarada.", []Port{in("input", "Entrada", "any", false)}, []Port{out("output", "Saída", "any")}),
		card("model", "Modelo IA", "IA", "Executa um modelo por um adaptador de provider.", []Port{in("prompt", "Prompt", "prompt", true)}, []Port{out("response", "Resposta", "model_response")}),
		card("validate", "Validar", "Validação", "Valida uma lista JSON de achados e, opcionalmente, seus caminhos nos arquivos buscados.", []Port{in("response", "Resposta", "model_response", true), in("files", "Arquivos buscados", "files", false)}, []Port{out("valid", "Resposta válida", "validated_response"), out("invalid", "Resposta inválida", "error")}),
		card("response_filter", "Filtrar resposta", "Validação", "Remove resultados inválidos, duplicados ou abaixo da política.", []Port{in("response", "Resposta validada", "validated_response", true)}, []Port{out("comments", "Comentários", "list")}),
		card("consolidate", "Consolidar", "Resultado", "Consolida comentários e produz uma revisão.", []Port{{Key: "comments", Label: "Comentários", Contract: "list", Required: true, CollectAll: true}}, []Port{out("review", "Review consolidada", "review")}),
		card("format", "Formatar", "Resultado", "Formata uma revisão para o destino configurado.", []Port{in("review", "Review", "review", true)}, []Port{out("formatted", "Resultado formatado", "formatted_review")}),
		card("publish", "Publicar no Gitea", "Saída", "Publica uma revisão formatada por uma integração Gitea ativa, com idempotência durável.", []Port{in("formatted_review", "Review formatada", "formatted_review", true)}, []Port{out("receipt", "Comprovante", "publication")}),
		card("log", "Log", "Infraestrutura", "Registra dados sanitizados para observabilidade.", []Port{in("input", "Entrada", "any", false)}, []Port{out("output", "Saída", "any")}),
		card("cache", "Cache", "Infraestrutura", "Lê ou grava dados efêmeros por uma chave configurada.", []Port{in("value", "Valor", "any", false)}, []Port{out("value", "Valor", "any")}),
		CardType{Key: "error_control", Name: "Controle de erro", Category: "Infraestrutura", Description: "Recebe um erro roteado e encerra, continua ou produz um fallback.", Inputs: []Port{in("error", "Erro", "error", true)}, Outputs: []Port{out("recovered", "Resultado de fallback", "any")}},
	)
}
