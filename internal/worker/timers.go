package worker

import (
	"time"

	v8 "github.com/tommie/v8go"
)

// setupTimers registers Go-backed setTimeout/setInterval/clearTimeout/clearInterval.
// Unlike the previous JS microtask-based implementation, these use real wall-clock
// delays via the eventLoop. Timer callbacks fire during eventLoop.drain() which is
// called by Engine.Execute after the fetch handler returns.
func setupTimers(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	// setTimeout(fn, delay) -> timerID
	setTimeoutFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 1 || !args[0].IsFunction() {
			val, _ := v8.NewValue(iso, int32(0))
			return val
		}
		fn, err := args[0].AsFunction()
		if err != nil {
			val, _ := v8.NewValue(iso, int32(0))
			return val
		}
		var delay time.Duration
		if len(args) > 1 {
			delay = time.Duration(args[1].Int32()) * time.Millisecond
		}
		id := el.setTimeout(fn, delay)
		val, _ := v8.NewValue(iso, int32(id))
		return val
	})
	ctx.Global().Set("setTimeout", setTimeoutFT.GetFunction(ctx))

	// clearTimeout(id)
	clearTimeoutFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) > 0 {
			el.clearTimer(int(args[0].Int32()))
		}
		return v8.Undefined(iso)
	})
	ctx.Global().Set("clearTimeout", clearTimeoutFT.GetFunction(ctx))

	// setInterval(fn, interval) -> timerID
	setIntervalFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 1 || !args[0].IsFunction() {
			val, _ := v8.NewValue(iso, int32(0))
			return val
		}
		fn, err := args[0].AsFunction()
		if err != nil {
			val, _ := v8.NewValue(iso, int32(0))
			return val
		}
		interval := 10 * time.Millisecond // minimum interval
		if len(args) > 1 && args[1].Int32() > 0 {
			interval = time.Duration(args[1].Int32()) * time.Millisecond
		}
		id := el.setInterval(fn, interval)
		val, _ := v8.NewValue(iso, int32(id))
		return val
	})
	ctx.Global().Set("setInterval", setIntervalFT.GetFunction(ctx))

	// clearInterval(id)  Esame semantics as clearTimeout.
	clearIntervalFT := v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) > 0 {
			el.clearTimer(int(args[0].Int32()))
		}
		return v8.Undefined(iso)
	})
	ctx.Global().Set("clearInterval", clearIntervalFT.GetFunction(ctx))

	return nil
}
