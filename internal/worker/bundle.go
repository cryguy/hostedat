package worker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

// DataDir is the base directory for cached polyfills. Defaults to "./data".
var DataDir = "./data"

// nodeCompatModules lists Node.js built-in modules that unenv provides polyfills for.
// Each entry maps to unenv/runtime/node/{name}/index.mjs.
var nodeCompatModules = []string{
	"async_hooks",
	"buffer",
	"crypto",
	"events",
	"fs",
	"http",
	"https",
	"module",
	"net",
	"os",
	"path",
	"process",
	"stream",
	"string_decoder",
	"url",
	"util",
}

var (
	resolvedUnenvPath string
	resolveUnenvOnce  sync.Once
)

// BundleWorkerScript uses esbuild to bundle a worker's _worker.js entry point
// with all its imports into a single self-contained script. This enables
// ES module import/export support for worker scripts.
//
// If the source doesn't contain any import statements, it's returned as-is
// to avoid unnecessary processing.
func BundleWorkerScript(deployPath string) (string, error) {
	entryPoint := filepath.Join(deployPath, "_worker.js")

	source, err := os.ReadFile(entryPoint)
	if err != nil {
		return "", fmt.Errorf("reading _worker.js: %w", err)
	}

	src := string(source)

	// Skip bundling if there are no import statements.
	if !needsBundling(src) {
		return src, nil
	}

	opts := esbuild.BuildOptions{
		EntryPoints:   []string{entryPoint},
		AbsWorkingDir: deployPath,
		Bundle:        true,
		Format:        esbuild.FormatESModule,
		Write:         false,
		Platform:      esbuild.PlatformBrowser,
		Target:        esbuild.ES2022,
		TreeShaking:   esbuild.TreeShakingFalse,
	}

	// Add Node.js compat aliases if unenv is available.
	if unenvDir := findUnenvPath(); unenvDir != "" {
		aliases := make(map[string]string, len(nodeCompatModules)*2)
		for _, mod := range nodeCompatModules {
			polyfill := filepath.Join(unenvDir, "runtime", "node", mod, "index.mjs")
			aliases["node:"+mod] = polyfill
			aliases[mod] = polyfill
		}
		opts.Alias = aliases
		// Add node_modules path so esbuild can resolve unenv's own deps (pathe, consola, etc.).
		opts.NodePaths = []string{filepath.Join(unenvDir, "..")}
	}

	result := esbuild.Build(opts)

	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, e.Text)
		}
		return "", fmt.Errorf("bundling _worker.js: %s", strings.Join(msgs, "; "))
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("bundling produced no output")
	}

	return string(result.OutputFiles[0].Contents), nil
}

// needsBundling checks if a script contains import statements that
// require bundling. Simple scripts without imports can skip this step.
func needsBundling(source string) bool {
	return strings.Contains(source, "import ") ||
		strings.Contains(source, "import{") ||
		strings.Contains(source, "import(") ||
		strings.Contains(source, "from 'node:") ||
		strings.Contains(source, "from \"node:") ||
		strings.Contains(source, "require(")
}

// findUnenvPath returns the absolute path to the unenv package directory,
// or an empty string if unenv is not available. The result is cached.
//
// It first checks the HOSTEDAT_UNENV_PATH env var, then auto-downloads
// unenv and its dependencies from the npm registry if needed.
func findUnenvPath() string {
	resolveUnenvOnce.Do(func() {
		// Allow override via environment variable (useful for tests and custom installs).
		if envPath := os.Getenv("HOSTEDAT_UNENV_PATH"); envPath != "" {
			if info, err := os.Stat(filepath.Join(envPath, "runtime", "node")); err == nil && info.IsDir() {
				resolvedUnenvPath = envPath
			}
			return
		}

		// Auto-download unenv polyfills if not cached.
		unenvDir, err := EnsureUnenv(DataDir)
		if err != nil {
			log.Printf("WARNING: failed to ensure unenv polyfills: %v", err)
			return
		}
		resolvedUnenvPath = unenvDir
	})
	return resolvedUnenvPath
}

// ResetUnenvCache clears the cached unenv path (used in tests).
func ResetUnenvCache() {
	resolveUnenvOnce = sync.Once{}
	resolvedUnenvPath = ""
}
