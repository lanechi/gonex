// Package cookie defines cookie configuration shared by the framework.
package cookie

import (
	"net/http"
	"time"
)

// Options controls a response cookie's security and lifetime flags.
type Options struct {
	Path     string
	Domain   string
	MaxAge   int
	Expires  time.Time
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
}
