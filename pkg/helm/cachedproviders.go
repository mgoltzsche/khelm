package helm

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/getter"
)

func withCachedHTTPGetter(cacheDir string, getters []getter.Provider) []getter.Provider {
	httpSchemes := []string{"http", "https"}
	for i := range getters {
		if containsAny(getters[i].Schemes, httpSchemes) {
			getters[i] = getter.Provider{
				Schemes: httpSchemes,
				New:     newCachedHTTPGetterConstructor(cacheDir),
			}
			break
		}
	}
	return getters
	/*getters := make([]getter.Provider, 1, len(fallback)+1)
	getters[0] = getter.Provider{
		Schemes: httpSchemes,
		New:     newCachedHTTPGetterConstructor(cacheDir),
	}
	getters = append(getters, fallback...)
	return getters*/
}

func containsAny(a []string, b []string) bool {
	for _, ae := range a {
		for _, be := range b {
			if ae == be {
				return true
			}
		}
	}
	return false
}

func newCachedHTTPGetterConstructor(cacheDir string) getter.Constructor {
	return func(o ...getter.Option) (getter.Getter, error) {
		httpGetter, err := getter.NewHTTPGetter()
		if err != nil {
			return nil, err
		}
		return &cachedHTTPGetter{cacheDir, httpGetter}, nil
	}
}

type cachedHTTPGetter struct {
	cacheDir   string
	httpGetter getter.Getter
}

func (g *cachedHTTPGetter) Get(location string, options ...getter.Option) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	u, err := url.Parse(location)
	if err != nil {
		return buf, fmt.Errorf("get: %w", err)
	}
	if u.Path == "" {
		return buf, fmt.Errorf("get %s: empty path in URL", location)
	}
	if u.RawQuery != "" {
		log.Printf("WARNING: Disabling cache since query params provided to getter with URL %s", location)
		return g.httpGetter.Get(location, options...)
	}

	// Try reading file from cache
	path := filepath.Clean(filepath.FromSlash(u.Path))
	if strings.Contains(path, "..") {
		return buf, fmt.Errorf("get %s: path %q points outside the cache dir", location, path)
	}
	cacheFilePath := filepath.Join(g.cacheDir, u.Host, path)
	cacheFile, err := os.Open(cacheFilePath)
	if err == nil {
		defer cacheFile.Close()
		_, err = io.Copy(buf, cacheFile)
		if err != nil {
			return buf, err
		}
		log.Printf("Using %s from cache", location)
		return buf, nil
	} else if !os.IsNotExist(err) {
		return buf, err
	}

	// Fetch URL since cache file does not exist
	buf, err = g.httpGetter.Get(location, options...)
	if err != nil {
		return buf, err
	}

	// Write cache
	if err = os.MkdirAll(filepath.Dir(cacheFilePath), 0755); err != nil {
		return buf, err
	}
	return buf, writeFileAtomic(bytes.NewReader(buf.Bytes()), cacheFilePath)
}
