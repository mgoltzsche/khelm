package helm

import (
	"errors"
	"path/filepath"

	securejoin "github.com/cyphar/filepath-securejoin"
)

func securePaths(paths []string, baseDir, rootDir string) (secured []string, err error) {
	secured = make([]string, len(paths))
	for i, path := range paths {
		if secured[i], err = securePath(path, baseDir, rootDir); err != nil {
			return
		}
	}
	return
}

func securePath(path, baseDir, rootDir string) (secured string, err error) {
	if rootDir == "" {
		return "", errors.New("no root dir provided")
	}
	if filepath.IsAbs(path) {
		if path, err = filepath.Rel(rootDir, path); err != nil {
			return
		}
	} else {
		if baseDir, err = filepath.Rel(rootDir, baseDir); err != nil {
			return
		}
		path = filepath.Join(baseDir, path)
	}
	return securejoin.SecureJoin(rootDir, path)
}