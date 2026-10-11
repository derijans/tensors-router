package proxy

import (
	"context"
	"sync/atomic"

	"tensors-router/internal/guardianmodels"
)

type guardianRedirectKey struct{}

func markGuardianRedirect(ctx context.Context) context.Context {
	return context.WithValue(ctx, guardianRedirectKey{}, true)
}

func contextIsGuardianRedirect(ctx context.Context) bool {
	redirected, _ := ctx.Value(guardianRedirectKey{}).(bool)
	return redirected
}

type guardianDirectory struct {
	store *guardianmodels.Store
	index atomic.Pointer[guardianmodels.Index]
}

func newGuardianDirectory(store *guardianmodels.Store) *guardianDirectory {
	directory := &guardianDirectory{store: store}
	directory.install(nil)
	return directory
}

func (directory *guardianDirectory) install(assignments []guardianmodels.Assignment) {
	directory.index.Store(guardianmodels.NewIndex(assignments))
}

func (directory *guardianDirectory) installed() []guardianmodels.Assignment {
	return directory.index.Load().Assignments()
}

func (directory *guardianDirectory) guardianOf(nodeID string, modelID string) (guardianmodels.Endpoint, bool) {
	return directory.index.Load().Resolve(nodeID, modelID)
}
