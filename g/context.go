package g

import (
	"context"

	"github.com/lanechi/gonex/ghttp"
)

// Context returns the framework context attached to the current controller
// context. It returns nil when called outside a framework HTTP request.
func Ctx(ctx context.Context) *ghttp.Context {
	return ghttp.FromContext(ctx)
}
