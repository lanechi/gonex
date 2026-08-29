package fs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanProjectRelative(relative string) (string, error) {
	clean := filepath.Clean(relative)
	if filepath.IsAbs(relative) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes project root", relative)
	}
	return clean, nil
}

func safeProjectPath(root, relative string) (string, error) {
	clean, err := cleanProjectRelative(relative)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, clean)
	current := root
	parts := strings.Split(clean, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("inspect path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q contains symlink", relative)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("path %q contains non-directory parent", relative)
		}
	}
	return path, nil
}

func projectRelativePath(root, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", err
	}
	return cleanProjectRelative(relative)
}

func createRootTempDir(root *os.Root, prefix string) (string, error) {
	if root == nil {
		return "", fmt.Errorf("project root is nil")
	}
	for range 100 {
		entropy := make([]byte, 8)
		if _, err := rand.Read(entropy); err != nil {
			return "", fmt.Errorf("generate temporary directory name: %w", err)
		}
		name := prefix + hex.EncodeToString(entropy)
		if err := root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("create unique temporary directory with prefix %q", prefix)
}

func pathOverlaps(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	return left == right || strings.HasPrefix(left, right+string(filepath.Separator)) || strings.HasPrefix(right, left+string(filepath.Separator))
}
