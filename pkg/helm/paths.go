package helm

import (
	"fmt"
	"path/filepath"
	"strings"
)

func absPaths(paths []string, baseDir string) []string {
	abs := make([]string, len(paths))
	for i, path := range paths {
		s_path := strings.Split(path, "://")
		if len(s_path) == 2 {
			abs[i] = fmt.Sprintf("%s://%s", s_path[0], absPath(s_path[1], baseDir))
			continue
		}
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
