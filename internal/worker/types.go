package worker

import "time"

// WorkerRequest represents an incoming HTTP request to a worker.
type WorkerRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// WorkerResponse represents the HTTP response from a worker.
type WorkerResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// WorkerResult wraps a response with execution metadata.
type WorkerResult struct {
	Response *WorkerResponse
	Logs     []LogEntry
	Error    error
	Duration time.Duration
}

// LogEntry is a single console.log/warn/error captured from a worker.
type LogEntry struct {
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// Env holds all bindings passed to the worker as the second argument.
type Env struct {
	Vars       map[string]string
	Secrets    map[string]string
	KVBindings map[string]string // binding name -> namespace ID
	Assets     AssetsFetcher
}

// AssetsFetcher is implemented by the static pipeline to handle env.ASSETS.fetch().
type AssetsFetcher interface {
	Fetch(req *WorkerRequest) (*WorkerResponse, error)
}
