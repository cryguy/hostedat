package workeradapter

import (
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/worker"
)

// Compile-time interface check.
var _ worker.SourceLoader = (*StorageSourceLoader)(nil)

// StorageSourceLoader implements worker.SourceLoader using the storage manager.
type StorageSourceLoader struct {
	Store *storage.Manager
}

// GetWorkerScript retrieves the worker JS source for the given site and deploy key.
func (s *StorageSourceLoader) GetWorkerScript(siteID, deployKey string) (string, error) {
	return s.Store.GetWorkerScript(siteID, deployKey)
}
