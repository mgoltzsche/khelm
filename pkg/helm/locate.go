package helm

import (
	"context"
	"fmt"
	"io/ioutil"
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
func locateChart(ctx context.Context, cfg *LoaderConfig, repos repositoryConfig, settings *cli.EnvSettings, getters getter.Providers) (string, error) {
	name := cfg.Chart

	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, errors.Errorf("path %q not found", name)
	}

	repoEntry, err := repos.Get(cfg.Repository)
	if err != nil {
		return "", err
	}

	cv, err := repos.ResolveChartVersion(ctx, name, cfg.Version, repoEntry.URL)
	if err != nil {
		return "", err
	}

	chartURL, err := repo.ResolveReferenceURL(repoEntry.URL, cv.URLs[0])
	if err != nil {
		return "", errors.Wrap(err, "failed to make chart URL absolute")
	}

	chartCacheDir := filepath.Join(settings.RepositoryCache, "khelm")
	cacheFile, err := cacheFilePath(chartURL, cv, chartCacheDir)
	if err != nil {
		return "", errors.Wrap(err, "derive chart cache file")
	}

	if err = ctx.Err(); err != nil {
		return "", err
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
		Out:     log.Writer(),
		Keyring: cfg.Keyring,
		Getters: getters,
		Options: []getter.Option{
			getter.WithBasicAuth(repoEntry.Username, repoEntry.Password),
			getter.WithTLSClientConfig(repoEntry.CertFile, repoEntry.KeyFile, repoEntry.CAFile),
			getter.WithInsecureSkipVerifyTLS(repoEntry.InsecureSkipTLSverify),
		},
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}
	if cfg.Verify {
		dl.Verify = downloader.VerifyAlways
	}

	destDir := filepath.Dir(cacheFile)
	destParentDir := filepath.Dir(destDir)
	if err = os.MkdirAll(destParentDir, 0750); err != nil {
		return "", errors.WithStack(err)
	}
	tmpDestDir, err := ioutil.TempDir(destParentDir, fmt.Sprintf(".tmp-%s-", filepath.Base(destDir)))
	if err != nil {
		return "", errors.WithStack(err)
	}

	interrupt := ctx.Done()
	done := make(chan error, 1)
	go func() (err error) {
		defer func() {
			done <- err
			close(done)
			if err != nil {
				_ = os.RemoveAll(tmpDestDir)
			}
		}()
		var chartFile string
		chartFile, _, err = dl.DownloadTo(chartURL, cv.Version, tmpDestDir)
		if err != nil {
			return errors.Wrapf(err, "failed to download chart %q with version %q", cfg.Chart, cv.Version)
		}
		chartFile, err = filepath.Abs(chartFile)
		if err != nil {
			return errors.WithStack(err)
		}
		return os.Rename(tmpDestDir, destDir)
	}()
	select {
	case err = <-done:
		return cacheFile, err
	case <-interrupt:
		_ = os.RemoveAll(tmpDestDir)
		return "", ctx.Err()
	}
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
	hostSegment := strings.ReplaceAll(u.Host, ":", "_")
	digestSegment := fmt.Sprintf("%s-%s-%s", cv.Name, cv.Version, cv.Digest[:16])
	return filepath.Join(cacheDir, hostSegment, filepath.Dir(path), digestSegment, filepath.Base(path)), nil
}
