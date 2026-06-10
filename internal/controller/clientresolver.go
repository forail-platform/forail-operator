package controller

import (
	"context"

	"github.com/forail-platform/forail-operator/internal/forailapi"
)

// clientFor returns the right Forail client for a CR's optional
// `spec.forailInstance` field. When the pool is nil or instanceName is
// empty the fallback (the controller's existing `.Forail` client) is used.
//
// This lets us roll out multi-cluster gradually: CRs that don't set
// `spec.forailInstance` continue to hit the default backend; CRs that do
// route through ForailInstance + Secret lookup.
func clientFor(ctx context.Context, pool *forailapi.ClientPool, fallback *forailapi.Client, namespace, instanceName string) (*forailapi.Client, error) {
	if instanceName == "" || pool == nil {
		return fallback, nil
	}
	return pool.For(ctx, namespace, instanceName)
}
