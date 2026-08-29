// Package g provides process-wide convenience helpers for Gonex applications.
package g

import (
	"context"

	"github.com/lanechi/gonex/ghttp"
)

// Ctx returns the framework context attached to the current controller
// context. It returns nil when called outside a framework HTTP request.
func Ctx(ctx context.Context) *ghttp.Context {
	return ghttp.FromContext(ctx)
}
