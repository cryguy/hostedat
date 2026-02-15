package worker

import (
	"strings"

	"github.com/fastschema/qjs"
)

// setupConsole replaces globalThis.console with a Go-backed version
// that captures output into the per-request log buffer.
func setupConsole(rt *qjs.Runtime) error {
	ctx := rt.Context()
	console := ctx.NewObject()

	levels := []string{"log", "info", "warn", "error", "debug"}
	for _, level := range levels {
		lvl := level // capture for closure
		fn := ctx.Function(func(this *qjs.This) (*qjs.Value, error) {
			c := this.Context()
			reqIDVal := c.Global().GetPropertyStr("__requestID")
			reqID := uint64(reqIDVal.Int64())
			reqIDVal.Free()

			args := this.Args()
			parts := make([]string, 0, len(args))
			for _, arg := range args {
				parts = append(parts, arg.String())
			}
			msg := strings.Join(parts, " ")
			addLog(reqID, lvl, msg)
			return c.NewUndefined(), nil
		}, false)
		console.SetPropertyStr(lvl, fn)
	}

	ctx.Global().SetPropertyStr("console", console)
	return nil
}
