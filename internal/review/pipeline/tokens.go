package pipeline

import "fmt"

func EstimateTokens(value string) int {
	if value == "" {
		return 0
	}
	// Conservative approximation for mixed source code and natural language.
	return len([]byte(value))/3 + 1
}

func SafeOutputTokens(cfg Config, prompt string, configured int) (int, error) {
	if configured <= 0 {
		return 0, fmt.Errorf("max_output_tokens deve ser positivo")
	}
	if cfg.ContextWindow <= 0 {
		return configured, nil
	}
	estimatedInput := EstimateTokens(prompt)
	if cfg.MaxInputTokens > 0 && estimatedInput > cfg.MaxInputTokens {
		return 0, fmt.Errorf("entrada excede max_input_tokens: estimated=%d max=%d", estimatedInput, cfg.MaxInputTokens)
	}
	margin := cfg.SafetyMarginTokens
	if margin <= 0 {
		margin = 2048
	}
	available := cfg.ContextWindow - estimatedInput - margin
	if available <= 0 {
		return 0, fmt.Errorf("entrada nao cabe no contexto: estimated_input_tokens=%d context_window=%d safety_margin=%d", estimatedInput, cfg.ContextWindow, margin)
	}
	if configured < available {
		return configured, nil
	}
	return available, nil
}
