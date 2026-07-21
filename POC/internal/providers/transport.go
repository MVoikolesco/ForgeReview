package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gitea-agents/internal/contracts"
)

// parseResult removes an optional Markdown JSON fence and decodes the common
// review contract returned by providers.
func parseResult(raw string) (contracts.Result, error) {
	raw = strings.TrimSpace(raw)
	if fence := strings.Index(raw, "```"); fence >= 0 {
		raw = strings.TrimSpace(raw[fence+3:])
		if strings.HasPrefix(raw, "json") {
			raw = strings.TrimSpace(raw[4:])
		}
		if end := strings.Index(raw, "```"); end >= 0 {
			raw = raw[:end]
		}
	}

	var result contracts.Result
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("provider returned invalid review JSON")
	}
	if result.Comments == nil {
		result.Comments = []contracts.Comment{}
	}

	return result, nil
}

// requestJSON sends one JSON request, applies optional bearer authentication,
// validates the HTTP status, and decodes the JSON response into target.
func requestJSON(
	ctx context.Context,
	client *http.Client,
	method string,
	endpoint string,
	key string,
	body any,
	target any,
) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("provider request failed with status %d", response.StatusCode)
	}

	return json.NewDecoder(response.Body).Decode(target)
}
