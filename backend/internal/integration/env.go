package integration

import (
	"fmt"
	"os"
)

// EnvironmentSecrets resolves exactly the environment variable selected by an
// integration's persisted secret reference. It has no provider-specific
// fallback names.
type EnvironmentSecrets struct{}

func (EnvironmentSecrets) Resolve(reference string) (string, error) {
	value, ok := os.LookupEnv(reference)
	if !ok || value == "" {
		return "", fmt.Errorf("secret environment variable %q is not set", reference)
	}
	return value, nil
}
