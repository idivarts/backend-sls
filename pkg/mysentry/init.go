// Package mysentry wraps Sentry error reporting for every Trendly lambda.
//
// Why this package exists instead of calling sentry-go directly: AWS freezes a
// Lambda execution environment the instant the handler returns, and Sentry's
// transport is asynchronous. Anything still in flight at that moment is lost
// silently — no error, no warning, just a missing event. Every wrapper below
// therefore flushes before returning, which is the one detail that makes
// Sentry-on-Lambda actually work.
//
// A missing SENTRY_DSN leaves every function here a no-op, so unconfigured
// stages and local runs cost nothing and never fail.
package mysentry

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// flushTimeout bounds how long an invocation waits for Sentry to drain. This is
// added latency on every single request, so keep it short — dropping a rare
// event is preferable to slowing down every call.
const flushTimeout = 2 * time.Second

var enabled bool

// Init configures the global Sentry client. Safe to call from several lambda
// entry points; sentry-go tolerates re-initialisation.
func Init() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		return
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: stage(),
		Release:     os.Getenv("SENTRY_RELEASE"),
		// Lambda containers are recycled constantly and traces would multiply
		// event volume for little benefit. Errors only, for now.
		EnableTracing: false,
	})
	if err != nil {
		// Never let observability wiring break the request path.
		log.Printf("[sentry] init failed, continuing without it: %v", err)
		return
	}

	enabled = true
}

// Enabled reports whether a DSN was configured and the client came up.
func Enabled() bool { return enabled }

func stage() string {
	if s := os.Getenv("STAGE"); s != "" {
		return s
	}
	return "dev"
}

// Capture reports an error that the caller is handling rather than returning —
// the "logged and swallowed" case that is otherwise invisible in production.
// tags may be nil.
func Capture(err error, tags map[string]string) {
	if !enabled || err == nil {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}

// CaptureMessage reports a non-error condition worth surfacing. tags may be nil.
func CaptureMessage(msg string, tags map[string]string) {
	if !enabled || msg == "" {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureMessage(msg)
	})
}

// Flush drains buffered events. Call it before any path that ends an
// invocation; the Wrap* helpers already do.
func Flush() {
	if !enabled {
		return
	}
	sentry.Flush(flushTimeout)
}

// recoverAndRepanic captures a panic and then lets it continue unwinding, so
// Lambda still marks the invocation failed and its retry / DLQ behaviour is
// unchanged. Deferred flushes registered earlier still run during the unwind.
func recoverAndRepanic() {
	r := recover()
	if r == nil {
		return
	}
	if enabled {
		sentry.CurrentHub().Recover(r)
	}
	panic(r)
}

// ── Handler wrappers ────────────────────────────────────────────────────────
//
// One per handler shape used across functions/*/main.go. Each captures panics
// and returned errors, then flushes before handing control back to Lambda.
//
// Note on hub usage: these share the global hub. That is safe here because a
// Lambda execution environment serves exactly one invocation at a time, and
// Capture/CaptureMessage confine tag mutations to a temporary scope.

// Wrap adapts handlers of the form func(ctx, Event) error — the SQS lambdas.
func Wrap[E any](h func(context.Context, E) error) func(context.Context, E) error {
	return func(ctx context.Context, event E) error {
		defer Flush()
		defer recoverAndRepanic()

		err := h(ctx, event)
		if err != nil {
			Capture(err, nil)
		}
		return err
	}
}

// WrapCtx adapts handlers of the form func(ctx) error — the scheduled/cron
// lambdas, which receive no event payload.
func WrapCtx(h func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		defer Flush()
		defer recoverAndRepanic()

		err := h(ctx)
		if err != nil {
			Capture(err, nil)
		}
		return err
	}
}

// WrapResult adapts handlers of the form func(ctx, Event) (Result, error) —
// used by the websocket lambda.
func WrapResult[E any, R any](h func(context.Context, E) (R, error)) func(context.Context, E) (R, error) {
	return func(ctx context.Context, event E) (R, error) {
		defer Flush()
		defer recoverAndRepanic()

		result, err := h(ctx, event)
		if err != nil {
			Capture(err, nil)
		}
		return result, err
	}
}

// WrapVoid adapts handlers of the form func(ctx, Event) with no return value —
// used by the video processing lambda. Only panics are reportable here, since
// the handler surfaces nothing else.
func WrapVoid[E any](h func(context.Context, E)) func(context.Context, E) {
	return func(ctx context.Context, event E) {
		defer Flush()
		defer recoverAndRepanic()

		h(ctx, event)
	}
}
