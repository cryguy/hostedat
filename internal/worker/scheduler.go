package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

// schedulerJS defines globalThis.scheduler with a wait() method
// that returns a Promise resolving after the given milliseconds.
const schedulerJS = `
globalThis.scheduler = {
	wait: function(ms) {
		return new Promise(function(resolve) {
			setTimeout(resolve, ms || 0);
		});
	},
};
`

// setupScheduler registers the scheduler global with wait().
func setupScheduler(_ *v8.Isolate, ctx *v8.Context, _ *eventLoop) error {
	if _, err := ctx.RunScript(schedulerJS, "scheduler.js"); err != nil {
		return fmt.Errorf("evaluating scheduler.js: %w", err)
	}
	return nil
}
