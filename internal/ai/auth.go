package ai

import (
	"net"
	"net/url"
	"strings"
)

// ConnectionRequiresAuthentication classifies the server-owned authentication
// mode. Ollama is unauthenticated only when its parsed endpoint is loopback.
func ConnectionRequiresAuthentication(providerName, authType, baseURL string) bool {
	if providerName != "ollama" {
		return !strings.EqualFold(strings.TrimSpace(authType), "none")
	}

	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return true
	}
	host := u.Hostname()
	return !strings.EqualFold(host, "localhost") && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())
}
