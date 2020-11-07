package helm

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/helm/pkg/downloader"
	"k8s.io/helm/pkg/getter"
	cli "k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/repo"
)

// locateChart fetches the chart if not present in cache and returns its path.
// (derived from https://github.com/helm/helm/blob/fc9b46067f8f24a90b52eba31e09b31e69011e93/pkg/action/install.go#L621 -
// with efficient caching)
func locateChart(cfg *LoaderConfig, repos repositoryConfig, settings *cli.EnvSettings, getters getter.Providers) (string, error) {
	name := cfg.Chart
	version := cfg.Version

	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, errors.Errorf("path %q not found", name)
	}

	repoEntry, err := repos.Get(cfg.Repository)
	if err != nil {
		return "", err
	}

	cv, err := repos.ResolveChartVersion(name, version, repoEntry.URL)
	if err != nil {
		return "", err
	}

	chartURL, err := repo.ResolveReferenceURL(repoEntry.URL, cv.URLs[0])
	if err != nil {
		return "", errors.Wrap(err, "failed to make chart URL absolute")
	}

	chartCacheDir := filepath.Join(settings.Home.Archive(), "khelm")
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
		log.Printf("Using chart %s from cache at %s", cfg.Chart, cacheFile)
		return cacheFile, nil
	}

	log.Printf("Downloading chart %s %s from repo %s", cfg.Chart, cv.Version, repoEntry.URL)

	dl := downloader.ChartDownloader{
		Out:      os.Stdout,
		Keyring:  cfg.Keyring,
		Getters:  getters,
		Username: repoEntry.Username,
		Password: repoEntry.Password,
		HelmHome: settings.Home,
	}
	if cfg.Verify {
		dl.Verify = downloader.VerifyAlways
	}

	destDir := filepath.Dir(cacheFile)
	if err = os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	filename, _, err := dl.DownloadTo(chartURL, cv.Version, destDir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to download chart %q with version %s", cfg.Chart, version)
	}
	lname, err := filepath.Abs(filename)
	if err != nil {
		return filename, errors.WithStack(err)
	}
	return lname, nil
}

func cacheFilePath(chartURL string, cv *repo.ChartVersion, cacheDir string) (string, error) {
	u, err := url.Parse(chartURL)
	if err != nil {
		return "", errors.Wrapf(err, "parse chart URL %q", chartURL)
	}
	if u.Path == "" {
		return "", errors.Errorf("parse chart URL %s: empty path in URL", chartURL)
	}

	// Try reading file from cache
	path := filepath.Clean(filepath.FromSlash(u.Path))
	if strings.Contains(path, "..") {
		return "", errors.Errorf("get %s: path %q points outside the cache dir", chartURL, path)
	}
	if len(cv.Digest) < 16 {
		return "", errors.Errorf("repo index entry for chart %q does not specify a digest", cv.Name)
	}
	digestSegment := fmt.Sprintf("%s-%s-%s", cv.Name, cv.Version, cv.Digest[:16])
	return filepath.Join(cacheDir, u.Host, filepath.Dir(path), digestSegment, filepath.Base(path)), nil
}
