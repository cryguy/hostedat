package worker

import (
	"time"

	"gorm.io/gorm"
)

// WorkerRequest represents an incoming HTTP request to a worker.
type WorkerRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// WorkerResponse represents the HTTP response from a worker.
type WorkerResponse struct {
	StatusCode   int
	Headers      map[string]string
	Body         []byte
	HasWebSocket bool // true when status is 101 and webSocket was set
}

// WorkerResult wraps a response with execution metadata.
type WorkerResult struct {
	Response  *WorkerResponse
	Logs      []LogEntry
	Error     error
	Duration  time.Duration
	WebSocket *WebSocketHandler // non-nil for WebSocket upgrade responses
}

// LogEntry is a single console.log/warn/error captured from a worker.
type LogEntry struct {
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// TailEvent represents a log event forwarded to a tail worker.
type TailEvent struct {
	ScriptName string     `json:"scriptName"`
	Logs       []LogEntry `json:"logs"`
	Exceptions []string   `json:"exceptions"`
	Outcome    string     `json:"outcome"`
	Timestamp  time.Time  `json:"timestamp"`
}

// Env holds all bindings passed to the worker as the second argument.
type Env struct {
	Vars            map[string]string
	Secrets         map[string]string
	KVBindings      map[string]string               // binding name -> namespace ID
	StorageBindings map[string]string               // binding name -> S3 bucket name
	QueueBindings   map[string]string               // binding name -> queue name
	D1Bindings      map[string]string               // binding name -> database ID
	ServiceBindings map[string]ServiceBindingConfig // binding name -> target config
	Assets          AssetsFetcher
	engine          *Engine  // set internally for service binding dispatch
	db              *gorm.DB // set internally for cache API and other global bindings
	d1DataDir       string   // set internally for D1 isolated database files
}

// AssetsFetcher is implemented by the static pipeline to handle env.ASSETS.fetch().
type AssetsFetcher interface {
	Fetch(req *WorkerRequest) (*WorkerResponse, error)
}
