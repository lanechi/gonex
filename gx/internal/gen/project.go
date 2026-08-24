package gen

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverProject finds the nearest go.mod from start upward. The generator
// intentionally does not assume that the current working directory is the
// module root.
func DiscoverProject(start string) (Project, error) {
	workingDir, err := filepath.Abs(start)
	if err != nil {
		return Project{}, fmt.Errorf("resolve project directory: %w", err)
	}
	info, err := os.Stat(workingDir)
	if err != nil {
		return Project{}, fmt.Errorf("stat project directory: %w", err)
	}
	if !info.IsDir() {
		workingDir = filepath.Dir(workingDir)
	}
	absolute := workingDir
	for {
		modFile := filepath.Join(absolute, "go.mod")
		if modulePath, readErr := readModulePath(modFile); readErr == nil {
			return Project{Root: absolute, WorkingDir: workingDir, ModulePath: modulePath}, nil
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			break
		}
		absolute = parent
	}
	return Project{}, fmt.Errorf("go.mod not found from %s or its parents", start)
}

func readModulePath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("module directive not found in %s", path)
}
