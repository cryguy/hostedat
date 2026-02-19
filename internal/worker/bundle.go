package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
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

	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{entryPoint},
		AbsWorkingDir: deployPath,
		Bundle:        true,
		Format:        esbuild.FormatESModule,
		Write:         false,
		Platform:      esbuild.PlatformBrowser,
		Target:        esbuild.ES2022,
		TreeShaking:   esbuild.TreeShakingFalse,
	})

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
		strings.Contains(source, "import(")
}
