package review

import (
	"context"
	"encoding/json"
)

type WorkflowCatalog struct {
	Contracts   []WorkflowContractCatalogItem   `json:"contracts"`
	Processors  []WorkflowProcessorCatalogItem  `json:"processors"`
	Entrypoints []WorkflowEntrypointCatalogItem `json:"entrypoints"`
	RouteModes  []string                        `json:"route_modes"`
	JoinModes   []WorkflowModeCatalogItem       `json:"join_modes"`
}

type WorkflowContractCatalogItem struct {
	Key                  string          `json:"key"`
	Version              int             `json:"version"`
	Schema               json.RawMessage `json:"schema"`
	SemanticValidatorKey string          `json:"semantic_validator_key"`
}

type WorkflowProcessorCatalogItem struct {
	Key          string          `json:"key"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	Executable   bool            `json:"executable"`
}

type WorkflowEntrypointCatalogItem struct {
	Key          string          `json:"key"`
	Name         string          `json:"name"`
	AdapterKey   string          `json:"adapter_key"`
	ConfigSchema json.RawMessage `json:"config_schema"`
}

type WorkflowModeCatalogItem struct {
	Key        string `json:"key"`
	Executable bool   `json:"executable"`
}

var registeredEntrypointAdapters = map[string]struct{}{
	"webhook": {},
	"api":     {},
	"manual":  {},
}

// WorkflowCatalog exposes only application-controlled contracts and adapters.
func (r *Repository) WorkflowCatalog(ctx context.Context) (WorkflowCatalog, error) {
	out := WorkflowCatalog{
		Contracts:   []WorkflowContractCatalogItem{},
		Processors:  []WorkflowProcessorCatalogItem{},
		Entrypoints: []WorkflowEntrypointCatalogItem{},
		RouteModes:  []string{"all_matches", "first_match"},
		JoinModes:   []WorkflowModeCatalogItem{{Key: "each_arrival", Executable: true}, {Key: "any", Executable: false}, {Key: "wait_all", Executable: false}},
	}
	rows, err := r.db.QueryContext(ctx, `SELECT key,version,schema_json,semantic_validator_key FROM stage_contracts WHERE is_system=1 ORDER BY key,version`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item WorkflowContractCatalogItem
		var schema string
		if err = rows.Scan(&item.Key, &item.Version, &schema, &item.SemanticValidatorKey); err != nil {
			rows.Close()
			return out, err
		}
		item.Schema = validCatalogJSON(schema)
		out.Contracts = append(out.Contracts, item)
	}
	if err = rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.db.QueryContext(ctx, `SELECT key,display_name,description,config_schema_json,is_executable FROM workflow_processor_catalog ORDER BY rowid`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item WorkflowProcessorCatalogItem
		var schema string
		var executable int
		if err = rows.Scan(&item.Key, &item.Name, &item.Description, &schema, &executable); err != nil {
			rows.Close()
			return out, err
		}
		item.ConfigSchema = validCatalogJSON(schema)
		item.Executable = executable != 0
		out.Processors = append(out.Processors, item)
	}
	if err = rows.Close(); err != nil {
		return out, err
	}

	rows, err = r.db.QueryContext(ctx, `SELECT key,display_name,adapter_key,config_schema_json FROM workflow_entrypoint_catalog WHERE is_enabled=1 ORDER BY rowid`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item WorkflowEntrypointCatalogItem
		var schema string
		if err = rows.Scan(&item.Key, &item.Name, &item.AdapterKey, &schema); err != nil {
			return out, err
		}
		if _, registered := registeredEntrypointAdapters[item.AdapterKey]; !registered {
			continue
		}
		item.ConfigSchema = validCatalogJSON(schema)
		out.Entrypoints = append(out.Entrypoints, item)
	}
	return out, rows.Err()
}

func validCatalogJSON(value string) json.RawMessage {
	if !json.Valid([]byte(value)) {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(value)
}
