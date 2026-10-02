package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPResolverUsesOnlyExplicitOperatorAddress(t *testing.T) {
	resolver := newIPResolver(func(key string) string {
		if key == "VOXIS_ADVERTISED_IP" {
			return " 10.0.0.5 "
		}
		return ""
	})

	internal, external := resolver.resolve(context.Background())
	require.Equal(t, "10.0.0.5", internal)
	require.Empty(t, external)
}

func TestIPResolverDoesNotProbeCloudMetadata(t *testing.T) {
	resolver := newIPResolver(func(string) string { return "" })

	internal, external := resolver.resolve(context.Background())
	require.Equal(t, "unknown", internal)
	require.Empty(t, external)
}
