package helm

import (
	"path/filepath"
)

func absPath(path, baseDir string) string {
	baseDir = filepath.FromSlash(baseDir)
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return path
}
