package helm

import (
	"path/filepath"
)

func absPaths(paths []string, baseDir string) []string {
	abs := make([]string, len(paths))
	for i, path := range paths {
		abs[i] = absPath(path, baseDir)
	}
	return abs
}

func absPath(path, baseDir string) string {
	baseDir = filepath.FromSlash(baseDir)
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return path
}
