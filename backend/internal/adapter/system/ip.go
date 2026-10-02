package system

import (
	"context"
	"strings"
	"sync"
)

// ipResolver returns the operator-supplied address for system statistics. OSS
// does not query cloud metadata services, which can expose deployment details.
type ipResolver struct {
	env func(string) string

	mu         sync.Mutex
	resolved   bool
	internalIP string
}

func newIPResolver(env func(string) string) *ipResolver {
	return &ipResolver{env: env}
}

func (r *ipResolver) resolve(_ context.Context) (internal, external string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.resolved {
		return r.internalIP, ""
	}
	advertised := strings.TrimSpace(r.env("VOXIS_ADVERTISED_IP"))
	if advertised == "" {
		return "unknown", ""
	}
	r.internalIP = advertised
	r.resolved = true
	return r.internalIP, ""
}
