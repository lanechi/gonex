package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func safeProjectPath(root, relative string) (string, error) {
	clean := filepath.Clean(relative)
	if filepath.IsAbs(relative) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes project root", relative)
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

func pathOverlaps(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	return left == right || strings.HasPrefix(left, right+string(filepath.Separator)) || strings.HasPrefix(right, left+string(filepath.Separator))
}
