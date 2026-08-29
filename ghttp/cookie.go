package ghttp

import (
	"fmt"
	"net/http"
	"time"
)

// CookieOptions controls a response cookie's security and lifetime flags.
type CookieOptions struct {
	Path     string
	Domain   string
	MaxAge   int
	Expires  time.Time
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
}

// CookieManager provides cookie operations for the current response.
type CookieManager struct {
	context *Context
}

func (ctx *Context) Cookie() *CookieManager {
	return &CookieManager{context: ctx}
}

func (manager *CookieManager) Get(name string) (string, error) {
	if manager == nil || manager.context == nil || manager.context.Request() == nil {
		return "", http.ErrNoCookie
	}
	cookie, err := manager.context.Request().Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func (manager *CookieManager) prepare(name, value string, options CookieOptions) (*http.Cookie, error) {
	if manager == nil || manager.context == nil || manager.context.gin == nil {
		return nil, http.ErrNotSupported
	}
	if manager.context.gin.Writer.Written() {
		return nil, fmt.Errorf("cannot set cookie %q after response headers were written", name)
	}
	responseCookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     options.Path,
		Domain:   options.Domain,
		MaxAge:   options.MaxAge,
		Expires:  options.Expires,
		Secure:   options.Secure,
		HttpOnly: options.HTTPOnly,
		SameSite: options.SameSite,
	}
	if responseCookie.SameSite == http.SameSiteNoneMode && !responseCookie.Secure {
		return nil, fmt.Errorf("SameSite=None cookie %q must be Secure", name)
	}
	if err := responseCookie.Valid(); err != nil {
		return nil, err
	}
	if serialized := responseCookie.String(); len(serialized) > 4096 {
		return nil, fmt.Errorf("cookie %q exceeds 4096 bytes", name)
	}
	return responseCookie, nil
}

func (manager *CookieManager) prepareDelete(name string, options CookieOptions) (*http.Cookie, error) {
	options.MaxAge = -1
	options.Expires = time.Unix(1, 0)
	return manager.prepare(name, "", options)
}

func (manager *CookieManager) writePrepared(cookie *http.Cookie) {
	if manager == nil || manager.context == nil || manager.context.gin == nil || cookie == nil {
		return
	}
	http.SetCookie(manager.context.gin.Writer, cookie)
}

func (manager *CookieManager) Set(name, value string, options CookieOptions) error {
	responseCookie, err := manager.prepare(name, value, options)
	if err != nil {
		return err
	}
	manager.writePrepared(responseCookie)
	return nil
}

func (manager *CookieManager) Delete(name string, options CookieOptions) error {
	responseCookie, err := manager.prepareDelete(name, options)
	if err != nil {
		return err
	}
	manager.writePrepared(responseCookie)
	return nil
}
