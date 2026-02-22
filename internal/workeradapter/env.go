package workeradapter

import (
	"github.com/cryguy/hostedat/internal/models"
	"github.com/cryguy/hostedat/internal/storage"
	"github.com/cryguy/worker"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

// BuildEnvOptions holds the dependencies needed to build a worker Env from the database.
type BuildEnvOptions struct {
	DB            *gorm.DB
	MinioClient   *minio.Client
	PresignClient *minio.Client
	PublicS3URL   string
	D1DataDir     string
	Dispatcher    worker.WorkerDispatcher
	Store         *storage.Manager
	Cache         *storage.SiteRulesCache
}

// BuildEnvFromDB loads environment variables, secrets, and all bindings
// for a site from the database, returning a worker.Env ready for execution.
func BuildEnvFromDB(opts BuildEnvOptions, siteID string, assets worker.AssetsFetcher) *worker.Env {
	env := &worker.Env{
		Vars:    make(map[string]string),
		Secrets: make(map[string]string),
		Assets:  assets,
		SiteID:  siteID,
	}

	if opts.D1DataDir != "" {
		env.D1DataDir = opts.D1DataDir
	}
	if opts.Dispatcher != nil {
		env.Dispatcher = opts.Dispatcher
	}

	// Cache API binding — site-scoped, always available.
	env.Cache = &GORMCacheStore{DB: opts.DB, SiteID: siteID}

	// Load environment variables and secrets.
	var envVars []models.WorkerEnvVar
	opts.DB.Where("site_id = ?", siteID).Find(&envVars)
	for _, ev := range envVars {
		if ev.Secret {
			env.Secrets[ev.Name] = ev.Value
		} else {
			env.Vars[ev.Name] = ev.Value
		}
	}

	// KV namespace bindings — each namespace gets a GORM-backed KVStore.
	var kvNamespaces []models.KVNamespace
	opts.DB.Where("site_id = ?", siteID).Find(&kvNamespaces)
	if len(kvNamespaces) > 0 {
		env.KV = make(map[string]worker.KVStore, len(kvNamespaces))
		for _, ns := range kvNamespaces {
			env.KV[ns.Name] = &GORMKVStore{DB: opts.DB, NamespaceID: ns.ID}
		}
	}

	// Storage bucket bindings — each bucket gets a minio-backed R2Store.
	var storageBuckets []models.StorageBucket
	opts.DB.Where("site_id = ?", siteID).Find(&storageBuckets)
	if len(storageBuckets) > 0 && opts.MinioClient != nil {
		env.Storage = make(map[string]worker.R2Store, len(storageBuckets))
		for _, b := range storageBuckets {
			env.Storage[b.Name] = &MinioR2Store{
				Client:        opts.MinioClient,
				PresignClient: opts.PresignClient,
				BucketName:    b.BucketName,
				PublicS3URL:   opts.PublicS3URL,
			}
		}
	}

	// D1 database bindings — prefixed with siteID for isolation.
	var d1Databases []models.D1Database
	opts.DB.Where("site_id = ?", siteID).Find(&d1Databases)
	if len(d1Databases) > 0 {
		env.D1Bindings = make(map[string]string, len(d1Databases))
		for _, d := range d1Databases {
			env.D1Bindings[d.Name] = siteID + "_" + d.DatabaseID
		}
	}

	// Durable Object namespace bindings — each gets a GORM-backed store.
	var doNamespaces []models.DurableObjectNamespace
	opts.DB.Where("site_id = ?", siteID).Find(&doNamespaces)
	if len(doNamespaces) > 0 {
		env.DurableObjects = make(map[string]worker.DurableObjectStore, len(doNamespaces))
		for _, ns := range doNamespaces {
			env.DurableObjects[ns.Name] = &GORMDurableObjectStore{DB: opts.DB}
		}
	}

	return env
}

// PlatformDispatcher implements worker.WorkerDispatcher by delegating to an Engine.
type PlatformDispatcher struct {
	Engine     *worker.Engine
	EnvFactory func(siteID string) *worker.Env
}

// Compile-time interface check.
var _ worker.WorkerDispatcher = (*PlatformDispatcher)(nil)

// Execute dispatches a worker request, optionally building the env from the factory.
func (pd *PlatformDispatcher) Execute(siteID, deployKey string, env *worker.Env, req *worker.WorkerRequest) *worker.WorkerResult {
	if env == nil {
		env = pd.EnvFactory(siteID)
	}
	return pd.Engine.Execute(siteID, deployKey, env, req)
}
