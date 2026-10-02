package port

import "context"

// HealthChecker defines the interface for checking service health
type HealthChecker interface {
	// Ping checks if the service is reachable and healthy within the given context
	Ping(ctx context.Context) error
}
