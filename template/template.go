// Package template provides the html/template manager used by Server.
package template

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Manager owns parsed html/template files and application functions.
type Manager struct {
	mu        sync.RWMutex
	root      string
	functions template.FuncMap
	templates *template.Template
	watcher   *fsnotify.Watcher
	watchStop chan struct{}
	watchDone chan struct{}
}

// New creates a template manager.
func New() *Manager { return &Manager{functions: make(template.FuncMap)} }

func (manager *Manager) SetRoot(root string) error {
	root = strings.TrimSpace(root)
	if root != "" {
		root = filepath.Clean(root)
	}
	manager.stopWatcher()
	manager.mu.Lock()
	manager.root = root
	manager.templates = nil
	manager.mu.Unlock()
	if err := manager.Reload(); err != nil {
		return err
	}
	return manager.startWatcher()
}

func (manager *Manager) AddFunc(name string, function any) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("template function name is required")
	}
	if function == nil {
		return fmt.Errorf("template function %q is nil", name)
	}
	functionValue := reflect.ValueOf(function)
	if functionValue.Kind() == reflect.Func && functionValue.IsNil() {
		return fmt.Errorf("template function %q is nil", name)
	}
	if err := validateFunc(name, function); err != nil {
		return err
	}
	manager.mu.Lock()
	manager.functions[name] = function
	rootConfigured := manager.root != ""
	manager.mu.Unlock()
	if !rootConfigured {
		return nil
	}
	return manager.Reload()
}

func validateFunc(name string, function any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("invalid template function %q: %v", name, recovered)
		}
	}()
	template.New("validation").Funcs(template.FuncMap{name: function})
	return nil
}

// Invalidate clears the parsed template cache. The next Execute call loads
// and parses the files again.
func (manager *Manager) Invalidate() {
	manager.mu.Lock()
	manager.templates = nil
	manager.mu.Unlock()
}

// Close stops the directory watcher. It is safe to call more than once.
func (manager *Manager) Close() error {
	return manager.stopWatcher()
}

func (manager *Manager) Reload() error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.root == "" {
		return fmt.Errorf("template root is not configured")
	}
	files := make([]string, 0)
	if err := filepath.WalkDir(manager.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".html") {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no HTML templates found under %s", manager.root)
	}
	parsed := template.New("root").Funcs(manager.functions)
	for _, path := range files {
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(manager.root, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		if _, err := parsed.New(name).Parse(string(contents)); err != nil {
			return fmt.Errorf("parse template %s: %w", name, err)
		}
	}
	manager.templates = parsed
	return nil
}

func (manager *Manager) Execute(writer io.Writer, name string, data any) error {
	manager.mu.RLock()
	templates := manager.templates
	manager.mu.RUnlock()
	if templates == nil {
		if err := manager.Reload(); err != nil {
			return err
		}
		manager.mu.RLock()
		templates = manager.templates
		manager.mu.RUnlock()
	}
	if templates.Lookup(name) == nil {
		return fmt.Errorf("template %q not found", name)
	}
	return templates.ExecuteTemplate(writer, name, data)
}

func (manager *Manager) startWatcher() error {
	manager.mu.RLock()
	root := manager.root
	manager.mu.RUnlock()
	if root == "" {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create template watcher: %w", err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return watcher.Add(path)
		}
		return nil
	}); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watch template root: %w", err)
	}

	manager.mu.Lock()
	if manager.root != root || manager.watcher != nil {
		manager.mu.Unlock()
		_ = watcher.Close()
		return nil
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	manager.watcher = watcher
	manager.watchStop = stop
	manager.watchDone = done
	manager.mu.Unlock()
	go manager.watchLoop(watcher, stop, done)
	return nil
}

func (manager *Manager) stopWatcher() error {
	manager.mu.Lock()
	watcher := manager.watcher
	stop := manager.watchStop
	done := manager.watchDone
	manager.watcher = nil
	manager.watchStop = nil
	manager.watchDone = nil
	manager.mu.Unlock()
	if watcher == nil {
		return nil
	}
	if stop != nil {
		close(stop)
	}
	err := watcher.Close()
	if done != nil {
		<-done
	}
	return err
}

func (manager *Manager) watchLoop(watcher *fsnotify.Watcher, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = watcher.Add(event.Name)
				}
			}
			manager.Invalidate()
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			// Watcher errors do not make the current cache unusable. A later
			// filesystem event or an explicit Reload can recover it.
		}
	}
}
