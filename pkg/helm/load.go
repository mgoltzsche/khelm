package helm

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/downloader"
	"k8s.io/helm/pkg/getter"
	cli "k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/resolver"
)

// load chart from local or remote location
func loadChart(ctx context.Context, cfg *ChartConfig, settings *cli.EnvSettings, getters getter.Providers) (*chart.Chart, error) {
	if cfg.Repository == "" {
		return buildAndLoadLocalChart(ctx, cfg, settings, getters)
	}
	return loadRemoteChart(ctx, cfg, settings, getters)
}

func loadRemoteChart(ctx context.Context, cfg *ChartConfig, settings *cli.EnvSettings, getters getter.Providers) (*chart.Chart, error) {
	repoURLs := map[string]struct{}{cfg.Repository: {}}
	repos, err := reposForURLs(repoURLs, cfg.AllowUnknownRepositories, settings, getters)
	if err != nil {
		return nil, err
	}
	defer repos.Close()
	isRange, err := isVersionRange(cfg.Version)
	if err != nil {
		return nil, err
	}
	if isRange {
		if err = repos.UpdateIndex(); err != nil {
			return nil, err
		}
	}
	chartPath, err := locateChart(&cfg.LoaderConfig, repos, settings, getters)
	if err != nil {
		return nil, err
	}
	return chartutil.Load(chartPath)
}

func buildAndLoadLocalChart(ctx context.Context, cfg *ChartConfig, settings *cli.EnvSettings, getters getter.Providers) (*chart.Chart, error) {
	if cfg.Chart != "." && !(strings.HasPrefix(cfg.Chart, "./") || cfg.Chart != ".." && strings.HasPrefix(cfg.Chart, "../")) {
		return nil, errors.Errorf("chart name must start with ./ if no repository specified but %q provided", cfg.Chart)
	}
	chartPath, err := securePath(cfg.Chart, cfg.BaseDir, cfg.RootDir)
	if err != nil {
		return nil, errors.Wrap(err, "no repository specified and invalid local chart path provided")
	}
	chartRequested, err := chartutil.Load(chartPath)
	if err != nil {
		return nil, err
	}

	localCharts := make([]localChart, 0, 1)
	dependencies := make([]*chartutil.Dependency, 0)
	needsRepoIndexUpdate, err := collectCharts(chartRequested, chartPath, cfg, &localCharts, &dependencies, 0)
	if err != nil {
		return nil, err
	}

	// Create (temporary) repository configuration that includes all dependencies
	repos, err := reposForDependencies(dependencies, cfg.AllowUnknownRepositories, settings, getters)
	if err != nil {
		return nil, errors.Wrap(err, "init temp repositories.yaml")
	}
	defer repos.Close()

	// Download/update repo indices
	if needsRepoIndexUpdate {
		err = repos.UpdateIndex()
	} else {
		err = repos.DownloadIndexFilesIfNotExist()
	}
	if err != nil {
		return nil, err
	}

	// Build local charts recursively
	needsReload, err := buildLocalCharts(localCharts, &cfg.LoaderConfig, repos, getters, settings)
	if err != nil {
		return nil, errors.Wrap(err, "build/fetch dependencies")
	}
	// Reload the chart with the updated Chart.lock file
	if needsReload {
		chartRequested, err = chartutil.Load(chartPath)
		if err != nil {
			return nil, errors.Wrapf(err, "failed reloading chart %s after dependency download", cfg.Chart)
		}
	}
	return chartRequested, nil
}

func isVersionRange(version string) (bool, error) {
	c, err := semver.NewConstraint(version)
	if err != nil {
		return true, err
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return true, nil
	}
	return v.String() != c.String(), nil
}

type localChart struct {
	Chart             *chart.Chart
	Path              string
	LocalDependencies bool
	Requirements      *chartutil.Requirements
	RequirementsLock  *chartutil.RequirementsLock
}

func collectCharts(chartRequested *chart.Chart, chartPath string, cfg *ChartConfig, localCharts *[]localChart, deps *[]*chartutil.Dependency, depth int) (needsRepoIndexUpdate bool, err error) {
	if depth > 20 {
		return false, errors.New("collect local charts recursively: max depth of 20 reached - cyclic dependency?")
	}
	meta := chartRequested.Metadata
	if meta == nil {
		return false, errors.Errorf("chart %s has no metadata", chartPath)
	}
	name := fmt.Sprintf("%s %s", meta.Name, meta.Version)
	lock, err := chartutil.LoadRequirementsLock(chartRequested)
	if err != nil && err != chartutil.ErrLockfileNotFound {
		return false, err
	}
	req, err := chartutil.LoadRequirements(chartRequested)
	if err != nil && err != chartutil.ErrRequirementsNotFound {
		return false, err
	}
	var reqDeps []*chartutil.Dependency
	if req != nil && req.Dependencies != nil {
		reqDeps = req.Dependencies
	}
	hasLocalDependencies := false
	for _, dep := range reqDeps {
		if strings.HasPrefix(dep.Repository, "file://") {
			hasLocalDependencies = true
			depChartPath := strings.TrimPrefix(dep.Repository, "file://")
			depChartPath, err := securePath(depChartPath, cfg.BaseDir, cfg.RootDir)
			if err != nil {
				return false, errors.Wrapf(err, "chart %s dependency %s repository", name, dep.Name)
			}
			depChart, err := chartutil.LoadDir(depChartPath)
			if err != nil {
				return false, err
			}
			needsUpdate, err := collectCharts(depChart, depChartPath, cfg, localCharts, deps, depth+1)
			if err != nil {
				return false, err
			}
			if needsUpdate {
				needsRepoIndexUpdate = true
			}
		} else if strings.HasPrefix(dep.Repository, "https://") || strings.HasPrefix(dep.Repository, "http://") {
			*deps = append(*deps, dep)
			if lock == nil {
				// Update repo index when remote dependencies present but no lock file
				needsRepoIndexUpdate = true
			}
		}
	}
	*localCharts = append(*localCharts, localChart{
		Chart:             chartRequested,
		Path:              chartPath,
		LocalDependencies: hasLocalDependencies,
		Requirements:      req,
		RequirementsLock:  lock,
	})
	return needsRepoIndexUpdate, nil
}

func buildLocalCharts(localCharts []localChart, cfg *LoaderConfig, repos repositoryConfig, getters getter.Providers, settings *cli.EnvSettings) (needsReload bool, err error) {
	for _, ch := range localCharts {
		if ch.Requirements == nil {
			continue
		}
		if err = renderutil.CheckDependencies(ch.Chart, ch.Requirements); err != nil || ch.LocalDependencies {
			needsReload = true
			meta := ch.Chart.Metadata
			if meta == nil {
				return false, errors.Errorf("chart %s has no metadata", ch.Path)
			}
			name := fmt.Sprintf("%s %s", meta.Name, meta.Version)
			log.Printf("Building/fetching chart %s dependencies", name)
			if lock := ch.RequirementsLock; lock != nil {
				if sum, err := resolver.HashReq(ch.Requirements); err != nil || sum != lock.Digest {
					return false, errors.Errorf("chart %s requirements.lock is out of sync with requirements.yaml", meta.Name)
				}
			}
			if err = buildChartDependencies(ch.Chart, ch.Path, cfg, repos, getters, settings); err != nil {
				return false, errors.Wrapf(err, "build chart %s", name)
			}
		}
	}
	return needsReload, nil
}

func dependencyFilePath(chartPath string, d *chartutil.Dependency) string {
	name := fmt.Sprintf("%s-%s.tgz", d.Name, d.Version)
	return filepath.Join(chartPath, "charts", name)
}

func buildChartDependencies(chartRequested *chart.Chart, chartPath string, cfg *LoaderConfig, repos repositoryConfig, getters getter.Providers, settings *cli.EnvSettings) error {
	man := &downloader.Manager{
		Out:        log.Writer(),
		ChartPath:  chartPath,
		Keyring:    cfg.Keyring,
		SkipUpdate: true,
		Getters:    getters,
		HelmHome:   settings.Home,
		Debug:      settings.Debug,
	}
	if cfg.Verify {
		man.Verify = downloader.VerifyAlways
	}
	// Workaround for leftover tmpcharts dir (which makes Build() fail)
	err := os.RemoveAll(filepath.Join(chartPath, "tmpcharts"))
	if err != nil {
		return err
	}

	// Downloads dependencies - respecting requirements.lock if present
	err = man.Build()
	if err != nil && errors.Cause(err).Error() == "entry not found" {
		if err = repos.UpdateIndex(); err == nil {
			err = man.Build()
		}
	}
	return err
}
