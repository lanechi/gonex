package ghttp

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/lanechi/gonex/static"
)

// StaticOptions controls cache headers, index serving, SPA fallback, and the
// file extensions a mount may serve. A nil Extensions slice uses the safe
// defaults; an empty non-nil slice denies all files.
type StaticOptions struct {
	CacheControl string
	Index        string
	SPAFallback  bool
	Extensions   []string
}

func (server *Server) Static(relative, root string) error {
	return server.mountStatic(relative, func() error { return static.Mount(server.engine, relative, root, static.Options{}) })
}

func (server *Server) StaticWithOptions(relative, root string, options StaticOptions) error {
	return server.mountStatic(relative, func() error { return static.Mount(server.engine, relative, root, static.Options(options)) })
}

func (server *Server) StaticFile(relative, path string) error {
	return server.mountStaticFile(func() error { return static.MountFile(server.engine, relative, path, static.Options{}) })
}

func (server *Server) StaticFileWithOptions(relative, path string, options StaticOptions) error {
	return server.mountStaticFile(func() error { return static.MountFile(server.engine, relative, path, static.Options(options)) })
}

func (server *Server) StaticFS(relative string, filesystem fs.FS) error {
	return server.mountStatic(relative, func() error { return static.MountFS(server.engine, relative, filesystem, static.Options{}) })
}

func (server *Server) StaticFSWithOptions(relative string, filesystem fs.FS, options StaticOptions) error {
	return server.mountStatic(relative, func() error { return static.MountFS(server.engine, relative, filesystem, static.Options(options)) })
}

func (server *Server) mountStatic(relative string, mount func() error) error {
	server.registrationMu.Lock()
	defer server.registrationMu.Unlock()
	if server.isRunning() {
		return ErrServerRunning
	}
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	rootMount := strings.Trim(relative, "/") == ""
	if rootMount && server.staticRootReady {
		return fmt.Errorf("root static handler is already configured")
	}
	if err := mount(); err != nil {
		return err
	}
	if rootMount {
		server.staticRootReady = true
	}
	return nil
}

func (server *Server) mountStaticFile(mount func() error) error {
	server.registrationMu.Lock()
	defer server.registrationMu.Unlock()
	if server.isRunning() {
		return ErrServerRunning
	}
	ginRouteRegistrationMu.Lock()
	defer ginRouteRegistrationMu.Unlock()
	return mount()
}
