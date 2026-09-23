package mysentry

import (
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// GinMiddleware returns the Sentry request middleware, enriching events with
// request context (method, URL, headers). Returns nil when Sentry is not
// configured, so callers must nil-check before Use().
//
// Registered after gin.Default()'s Recovery, so a panic is captured here and
// then re-raised for Recovery to turn into the same 500 it produces today.
func GinMiddleware() gin.HandlerFunc {
	if !enabled {
		return nil
	}

	return sentrygin.New(sentrygin.Options{
		// gin.Default() brings its own Recovery, which owns the HTTP response.
		Repanic: true,
		// Delivery is flushed once per invocation in apihandler.Handler rather
		// than per request — a Lambda serves one request per invocation, so
		// blocking here as well would just add latency twice.
		WaitForDelivery: false,
	})
}
