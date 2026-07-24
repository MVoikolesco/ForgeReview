package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	maxResponseSchemaBytes = 64 << 10
	maxResponseSchemaDepth = 32
)

// ResponseContract is an immutable contract offered by ForgeReview. A workflow
// stores a copy of Schema in its validate card, so executions remain versioned
// even when the catalog evolves.
type ResponseContract struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     int            `json:"version"`
	Schema      map[string]any `json:"schema"`
	Editable    bool           `json:"editable"`
}

var officialResponseContracts = []struct {
	key, name, description, schema string
}{
	{
		key: "review.findings.v1", name: "Achados de review",
		description: "Lista de problemas encontrados em linhas adicionadas dos arquivos alterados.",
		schema:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","items":{"type":"object","additionalProperties":false,"required":["path","line","comment","severity"],"properties":{"path":{"type":"string","minLength":1},"line":{"type":"integer","minimum":1},"comment":{"type":"string","minLength":1},"severity":{"type":"string","enum":["low","medium","high","critical"]}}}}`,
	},
	{
		key: "review.candidate-findings.v1", name: "Candidatos de review",
		description: "Candidatos estruturados que ainda exigem uma decisão independente antes da publicação.",
		schema:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","items":{"type":"object","additionalProperties":false,"required":["check_id","claim","scenario","impact","evidence","confidence","required_context","symbol","issue_type","affected_entity","path","line","comment","severity"],"properties":{"check_id":{"type":"string","minLength":1},"claim":{"type":"string","minLength":1},"scenario":{"type":"string","minLength":1},"impact":{"type":"string","minLength":1},"evidence":{"type":"array","minItems":1,"items":{"type":"string","minLength":1}},"confidence":{"type":"number","minimum":0,"maximum":1},"required_context":{"type":"array","items":{"type":"string","minLength":1}},"symbol":{"type":"string","minLength":1},"issue_type":{"type":"string","minLength":1},"affected_entity":{"type":"string","minLength":1},"path":{"type":"string","minLength":1},"line":{"type":"integer","minimum":1},"comment":{"type":"string","minLength":1},"severity":{"type":"string","enum":["low","medium","high","critical"]}}}}`,
	},
	{
		key: "generic.object.v1", name: "Objeto JSON",
		description: "Objeto JSON genérico para integrações e transformações estruturadas.",
		schema:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
	},
	{
		key: "generic.array.v1", name: "Lista JSON",
		description: "Lista JSON genérica para respostas estruturadas.",
		schema:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array"}`,
	},
	{
		key: "review.summary.v1", name: "Resumo de review",
		description: "Resumo estruturado com conclusão e principais pontos da análise.",
		schema:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["summary","highlights"],"properties":{"summary":{"type":"string","minLength":1},"highlights":{"type":"array","items":{"type":"string"}}}}`,
	},
}

func ResponseContracts() []ResponseContract {
	contracts := make([]ResponseContract, 0, len(officialResponseContracts))
	for _, item := range officialResponseContracts {
		var schema map[string]any
		if err := json.Unmarshal([]byte(item.schema), &schema); err != nil {
			panic("invalid built-in response contract: " + err.Error())
		}
		contracts = append(contracts, ResponseContract{
			Key: item.key, Name: item.name, Description: item.description,
			Version: 1, Schema: schema, Editable: false,
		})
	}
	return contracts
}

func responseContractSchema(key string) map[string]any {
	for _, contract := range ResponseContracts() {
		if contract.Key == key {
			return contract.Schema
		}
	}
	panic("unknown built-in response contract: " + key)
}

func responseSchemaFromConfig(config map[string]any) (any, bool) {
	if config == nil {
		return nil, false
	}
	schema, exists := config["response_schema"]
	return schema, exists
}

// validateResponseSchema compiles the schema and applies ForgeReview's resource
// limits. External references are rejected so validation never performs I/O.
func validateResponseSchema(value any) (*jsonschema.Schema, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("response schema is not valid JSON: %w", err)
	}
	if len(payload) > maxResponseSchemaBytes {
		return nil, fmt.Errorf("response schema exceeds %d bytes", maxResponseSchemaBytes)
	}
	var schema any
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err = decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("response schema is not valid JSON: %w", err)
	}
	if _, ok := schema.(map[string]any); !ok {
		return nil, fmt.Errorf("response schema must be a JSON object")
	}
	if err = inspectResponseSchema(schema, 0); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.LoadURL = func(string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema resources are disabled")
	}
	if err = compiler.AddResource("urn:forgereview:response-schema", bytes.NewReader(payload)); err != nil {
		return nil, fmt.Errorf("response schema is invalid: %w", err)
	}
	compiled, err := compiler.Compile("urn:forgereview:response-schema")
	if err != nil {
		return nil, fmt.Errorf("response schema is invalid: %w", err)
	}
	return compiled, nil
}

func inspectResponseSchema(value any, depth int) error {
	if depth > maxResponseSchemaDepth {
		return fmt.Errorf("response schema exceeds maximum depth %d", maxResponseSchemaDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if key == "$ref" {
				ref, ok := nested.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return fmt.Errorf("response schema contains an external $ref")
				}
			}
			if err := inspectResponseSchema(nested, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, nested := range typed {
			if err := inspectResponseSchema(nested, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
