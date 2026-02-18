package worker

import (
	"fmt"
	"strings"

	v8 "github.com/tommie/v8go"
)

// setupConsole replaces globalThis.console with a Go-backed version
// that captures output into the per-request log buffer.
func setupConsole(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	console, err := newJSObject(iso, ctx)
	if err != nil {
		return fmt.Errorf("creating console object: %w", err)
	}

	levels := []string{"log", "info", "warn", "error", "debug"}
	for _, level := range levels {
		lvl := level // capture for closure
		ft := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
			reqIDVal, _ := ctx.Global().Get("__requestID")
			var reqID uint64
			if reqIDVal != nil && !reqIDVal.IsUndefined() && !reqIDVal.IsNull() {
				reqID = uint64(reqIDVal.Integer())
			}

			args := info.Args()
			parts := make([]string, 0, len(args))
			for _, arg := range args {
				parts = append(parts, arg.String())
			}
			msg := strings.Join(parts, " ")
			addLog(reqID, lvl, msg)
			return v8.Undefined(iso)
		})
		console.Set(lvl, ft.GetFunction(ctx))
	}

	ctx.Global().Set("console", console)
	return nil
}
