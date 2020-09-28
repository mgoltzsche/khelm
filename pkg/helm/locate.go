package helm

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// locateChart fetches the chart if not present in cache and returns its path.
// (derived from https://github.com/helm/helm/blob/fc9b46067f8f24a90b52eba31e09b31e69011e93/pkg/action/install.go#L621 -
// with efficient caching)
func locateChart(cfg *LoadChartConfig, repos repositoryConfig, settings *cli.EnvSettings, getters getter.Providers) (string, error) {
	name := cfg.Chart
	version := cfg.Version

	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, fmt.Errorf("path %q not found", name)
	}

	cv, err := repos.ResolveChartVersion(name, version, cfg.Repository)
	if err != nil {
		return "", err
	}

	chartURL, err := repo.ResolveReferenceURL(cfg.Repository, cv.URLs[0])
	if err != nil {
		return "", errors.Wrap(err, "failed to make chart URL absolute")
	}

	chartCacheDir := filepath.Join(settings.RepositoryCache, "khelm")
	cacheFile, err := cacheFilePath(chartURL, cv, chartCacheDir)
	if err != nil {
		return "", errors.Wrap(err, "derive chart cache file")
	}

	if _, err = os.Stat(cacheFile); err == nil {
		if cfg.Verify {
			if _, err := downloader.VerifyChart(cacheFile, cfg.Keyring); err != nil {
				return "", err
			}
		}
		return cacheFile, nil
	}

	log.Printf("Downloading chart %s from repo %s", cfg.Chart, cfg.Repository)

	entry, err := repos.EntryByURL(cfg.Repository)
	if err != nil {
		return "", err
	}
	dl := downloader.ChartDownloader{
		Out:     os.Stdout,
		Keyring: cfg.Keyring,
		Getters: getters,
		Options: []getter.Option{
			getter.WithBasicAuth(entry.Username, entry.Password),
			getter.WithTLSClientConfig(entry.CertFile, entry.KeyFile, entry.CAFile),
			getter.WithInsecureSkipVerifyTLS(entry.InsecureSkipTLSverify),
		},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}
	if cfg.Verify {
		dl.Verify = downloader.VerifyAlways
	}

	destDir := filepath.Dir(cacheFile)
	if err = os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	filename, _, err := dl.DownloadTo(chartURL, version, destDir)
	if err != nil {
		return "", fmt.Errorf("failed to download chart %q with version %s: %w", cfg.Chart, version, err)
	}
	lname, err := filepath.Abs(filename)
	if err != nil {
		return filename, err
	}
	return lname, nil
}

func cacheFilePath(chartURL string, cv *repo.ChartVersion, cacheDir string) (string, error) {
	u, err := url.Parse(chartURL)
	if err != nil {
		return "", fmt.Errorf("parse chart URL %q: %w", chartURL, err)
	}
	if u.Path == "" {
		return "", fmt.Errorf("parse chart URL %s: empty path in URL", chartURL)
	}

	// Try reading file from cache
	path := filepath.Clean(filepath.FromSlash(u.Path))
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("get %s: path %q points outside the cache dir", chartURL, path)
	}
	digestSegment := fmt.Sprintf("%s-%s-%s", cv.Name, cv.Version, cv.Digest[:16])
	return filepath.Join(cacheDir, u.Host, filepath.Dir(path), digestSegment, filepath.Base(path)), nil
}
