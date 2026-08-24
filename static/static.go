// Package static provides safe directory, file, and fs.FS mounts.
package static

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Options controls cache headers, index serving, SPA fallback, and file types.
type Options struct {
	CacheControl string
	Index        string
	SPAFallback  bool
	// Extensions is the case-insensitive allowlist of served file extensions.
	// A nil list uses the safe default; an empty non-nil list denies every file.
	Extensions []string
}

var defaultExtensions = map[string]struct{}{
	".html": {}, ".htm": {}, ".js": {}, ".mjs": {}, ".css": {},
	".png": {}, ".apng": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".svg": {}, ".webp": {}, ".avif": {}, ".jxl": {}, ".ico": {}, ".bmp": {}, ".tif": {}, ".tiff": {},
	".woff": {}, ".woff2": {}, ".ttf": {}, ".otf": {}, ".eot": {},
	".wasm": {}, ".webmanifest": {},
}

// Mount mounts a local directory below a URL prefix.
func Mount(engine *gin.Engine, relative, root string, options Options) error {
	if engine == nil {
		return fmt.Errorf("static engine is nil")
	}
	if !validMountPath(relative) {
		return fmt.Errorf("static URI %q must be an absolute clean URI path", relative)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("open static root %q: %w", root, err)
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		return fmt.Errorf("open static root %q: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("static root %q is not a directory", root)
	}
	prefix := strings.TrimRight(relative, "/")
	if prefix == "" {
		prefix = "/"
	}
	index := staticIndex(options)
	allowed := extensionAllowlist(options)
	handler := func(context *gin.Context) {
		if context.Request.Method != http.MethodGet && context.Request.Method != http.MethodHead {
			context.Status(http.StatusNotFound)
			return
		}
		requestedPath, ok := requestPath(context, prefix)
		if !ok {
			context.Status(http.StatusNotFound)
			return
		}
		if requestedPath == "" {
			requestedPath = index
		}
		if path.Ext(requestedPath) != "" && !allowedFile(requestedPath, allowed) {
			context.Status(http.StatusNotFound)
			return
		}
		if localPathEscapes(resolvedRoot, requestedPath) {
			context.Status(http.StatusNotFound)
			return
		}
		candidate, info, ok := localFile(resolvedRoot, requestedPath, index)
		if !ok || !allowedFile(candidate, allowed) {
			serveFallback(context, resolvedRoot, index, allowed, options.SPAFallback, options.CacheControl)
			return
		}
		serveLocalFile(context, candidate, info, options.CacheControl)
	}
	if prefix == "/" {
		engine.NoRoute(handler)
		return nil
	}
	return registerHandlers(engine, wildcardPattern(prefix), handler)
}

// MountFile mounts one file below a URL path.
func MountFile(engine *gin.Engine, relative, filePath string, options Options) error {
	if engine == nil {
		return fmt.Errorf("static engine is nil")
	}
	if !validMountPath(relative) || strings.HasSuffix(relative, "/") {
		return fmt.Errorf("static file URI %q must be an absolute file URI path", relative)
	}
	fileInfo, err := os.Lstat(filePath)
	if err != nil {
		return fmt.Errorf("open static file %q: %w", filePath, err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("static file %q must not be a symbolic link", filePath)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("static file %q is a directory", filePath)
	}
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return fmt.Errorf("resolve static file %q: %w", filePath, err)
	}
	allowed := extensionAllowlist(options)
	handler := func(context *gin.Context) {
		if !validEscapedURLPath(context.Request.URL.EscapedPath()) || !allowedFile(resolvedPath, allowed) {
			context.Status(http.StatusNotFound)
			return
		}
		if options.CacheControl != "" {
			context.Header("Cache-Control", options.CacheControl)
		}
		http.ServeFile(context.Writer, context.Request, resolvedPath)
	}
	return registerHandlers(engine, relative, handler)
}

// MountFS mounts an io/fs filesystem, including an embed.FS.
func MountFS(engine *gin.Engine, relative string, filesystem fs.FS, options Options) error {
	if engine == nil {
		return fmt.Errorf("static engine is nil")
	}
	if !validMountPath(relative) {
		return fmt.Errorf("static FS URI %q must be an absolute clean URI path", relative)
	}
	if filesystem == nil {
		return fmt.Errorf("static filesystem is nil")
	}
	prefix := strings.TrimRight(relative, "/")
	if prefix == "" {
		prefix = "/"
	}
	index := staticIndex(options)
	allowed := extensionAllowlist(options)
	fileServer := http.FileServer(http.FS(filesystem))
	handler := func(context *gin.Context) {
		if context.Request.Method != http.MethodGet && context.Request.Method != http.MethodHead {
			context.Status(http.StatusNotFound)
			return
		}
		requestedPath, ok := requestPath(context, prefix)
		if !ok {
			context.Status(http.StatusNotFound)
			return
		}
		if requestedPath == "" {
			requestedPath = index
		}
		if path.Ext(requestedPath) != "" && !allowedFile(requestedPath, allowed) {
			context.Status(http.StatusNotFound)
			return
		}
		requestedPath, info, ok := fsFile(filesystem, requestedPath, index)
		if !ok || !allowedFile(requestedPath, allowed) {
			serveFSFallback(context, filesystem, index, allowed, options.SPAFallback, options.CacheControl, fileServer)
			return
		}
		if info.IsDir() {
			context.Status(http.StatusNotFound)
			return
		}
		serveFSFile(context, fileServer, requestedPath, options.CacheControl)
	}
	if prefix == "/" {
		engine.NoRoute(handler)
		return nil
	}
	return registerHandlers(engine, wildcardPattern(prefix), handler)
}

func staticIndex(options Options) string {
	if options.Index == "" || !validFilePath(options.Index) {
		return "index.html"
	}
	return options.Index
}

func extensionAllowlist(options Options) map[string]struct{} {
	if options.Extensions == nil {
		return defaultExtensions
	}
	allowed := make(map[string]struct{}, len(options.Extensions))
	for _, extension := range options.Extensions {
		extension = strings.TrimSpace(strings.ToLower(extension))
		if extension == "" {
			continue
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		allowed[extension] = struct{}{}
	}
	return allowed
}

func allowedFile(name string, allowed map[string]struct{}) bool {
	_, ok := allowed[strings.ToLower(filepath.Ext(name))]
	return ok
}

func requestPath(context *gin.Context, prefix string) (string, bool) {
	escapedPath := context.Request.URL.EscapedPath()
	if !validEscapedURLPath(escapedPath) {
		return "", false
	}
	if prefix != "/" {
		if !strings.HasPrefix(context.Request.URL.Path, prefix) {
			return "", false
		}
		escapedPath = strings.TrimPrefix(escapedPath, prefix)
	}
	requestedPath := strings.TrimPrefix(escapedPath, "/")
	requestedPath = strings.TrimSuffix(requestedPath, "/")
	if requestedPath == "" {
		return "", true
	}
	return safeFilePath(requestedPath)
}

func validEscapedURLPath(escapedPath string) bool {
	decoded, err := unescapePath(escapedPath)
	if err != nil || !strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "\\") {
		return false
	}
	return decoded == "/" || path.Clean(decoded) == decoded ||
		(strings.HasSuffix(decoded, "/") && path.Clean(decoded) == strings.TrimSuffix(decoded, "/"))
}

func unescapePath(value string) (string, error) {
	for range 3 {
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return "", err
		}
		if decoded == value {
			return decoded, nil
		}
		value = decoded
	}
	return value, nil
}

func safeFilePath(value string) (string, bool) {
	decoded, err := unescapePath(value)
	if err != nil || !validFilePath(decoded) {
		return "", false
	}
	return decoded, true
}

func validFilePath(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !path.IsAbs(value) && !filepath.IsAbs(value) && path.Clean(value) == value && fs.ValidPath(value)
}

func validMountPath(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\?#") || path.Clean(value) != value {
		return false
	}
	decoded, err := unescapePath(value)
	return err == nil && decoded == value
}

func localFile(root, requestedPath, index string) (string, fs.FileInfo, bool) {
	candidate := filepath.Join(root, filepath.FromSlash(requestedPath))
	if !pathWithinRoot(root, candidate) {
		return "", nil, false
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", nil, false
	}
	if info.IsDir() {
		candidate = filepath.Join(candidate, index)
		if !pathWithinRoot(root, candidate) {
			return "", nil, false
		}
		info, err = os.Stat(candidate)
		if err != nil || info.IsDir() {
			return "", nil, false
		}
	}
	return candidate, info, true
}

func localPathEscapes(root, requestedPath string) bool {
	candidate := filepath.Join(root, filepath.FromSlash(requestedPath))
	for {
		if info, err := os.Lstat(candidate); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return true
			}
			return !pathWithinRoot(root, candidate)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return true
		}
		candidate = parent
	}
}

func fsFile(filesystem fs.FS, requestedPath, index string) (string, fs.FileInfo, bool) {
	if !validFilePath(requestedPath) || fsPathHasSymlink(filesystem, requestedPath) {
		return "", nil, false
	}
	info, err := fs.Stat(filesystem, requestedPath)
	if err != nil {
		return "", nil, false
	}
	if info.IsDir() {
		requestedPath = path.Join(requestedPath, index)
		if !validFilePath(requestedPath) || fsPathHasSymlink(filesystem, requestedPath) {
			return "", nil, false
		}
		info, err = fs.Stat(filesystem, requestedPath)
		if err != nil || info.IsDir() {
			return "", nil, false
		}
	}
	return requestedPath, info, true
}

func fsPathHasSymlink(filesystem fs.FS, name string) bool {
	linkFS, ok := filesystem.(fs.ReadLinkFS)
	if !ok {
		return false
	}
	for current := name; current != "."; current = path.Dir(current) {
		info, err := linkFS.Lstat(current)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			return true
		}
	}
	return false
}

func serveFallback(context *gin.Context, root, index string, allowed map[string]struct{}, enabled bool, cacheControl string) {
	if !enabled || !allowedFile(index, allowed) {
		context.Status(http.StatusNotFound)
		return
	}
	candidate, info, ok := localFile(root, index, index)
	if !ok || !allowedFile(candidate, allowed) {
		context.Status(http.StatusNotFound)
		return
	}
	serveLocalFile(context, candidate, info, cacheControl)
}

func serveLocalFile(context *gin.Context, candidate string, _ fs.FileInfo, cacheControl string) {
	if cacheControl != "" {
		context.Header("Cache-Control", cacheControl)
	}
	http.ServeFile(context.Writer, context.Request, candidate)
}

func serveFSFallback(context *gin.Context, filesystem fs.FS, index string, allowed map[string]struct{}, enabled bool, cacheControl string, fileServer http.Handler) {
	if !enabled || !allowedFile(index, allowed) {
		context.Status(http.StatusNotFound)
		return
	}
	requestedPath, _, ok := fsFile(filesystem, index, index)
	if !ok || !allowedFile(requestedPath, allowed) {
		context.Status(http.StatusNotFound)
		return
	}
	serveFSFile(context, fileServer, requestedPath, cacheControl)
}

func serveFSFile(context *gin.Context, fileServer http.Handler, requestedPath, cacheControl string) {
	if cacheControl != "" {
		context.Header("Cache-Control", cacheControl)
	}
	request := context.Request.Clone(context.Request.Context())
	request.URL.Path = "/" + requestedPath
	fileServer.ServeHTTP(context.Writer, request)
}

func registerHandlers(engine *gin.Engine, routePath string, handler gin.HandlerFunc) (err error) {
	for _, route := range engine.Routes() {
		if route.Path == routePath && (route.Method == http.MethodGet || route.Method == http.MethodHead) {
			return fmt.Errorf("static route %s %s is already registered", route.Method, routePath)
		}
	}
	if err := validateRouteRegistration(engine, routePath); err != nil {
		return err
	}
	engine.GET(routePath, handler)
	engine.HEAD(routePath, handler)
	return nil
}

// validateRouteRegistration mirrors both registrations in a temporary Gin
// engine before changing the caller's route tree. Gin can reject HEAD after a
// GET has already been accepted, so validating the pair together keeps mounts
// atomic.
func validateRouteRegistration(engine *gin.Engine, routePath string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register static route %s: %v", routePath, recovered)
		}
	}()
	temporary := gin.New()
	placeholder := func(*gin.Context) {}
	for _, route := range engine.Routes() {
		temporary.Handle(route.Method, route.Path, placeholder)
	}
	temporary.GET(routePath, placeholder)
	temporary.HEAD(routePath, placeholder)
	return nil
}

func wildcardPattern(prefix string) string {
	if prefix == "/" {
		return "/*filepath"
	}
	return prefix + "/*filepath"
}

func pathWithinRoot(root, candidate string) bool {
	rootPath, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	candidatePath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootPath, candidatePath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}
