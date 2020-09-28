package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
)

// Render manifest from helm chart configuration (shorthand)
func Render(ctx context.Context, cfg *GeneratorConfig, writer io.Writer) (err error) {
	settings := cli.New()
	getters := getter.All(settings)
	//cacheDir := filepath.Join(settings.RepositoryCache, "khelm")
	//cachedGetters := withCachedHTTPGetter(cacheDir, getters)

	// Load values
	vals, err := loadValues(&cfg.RenderConfig, getters)
	if err != nil {
		return err
	}

	// Create helm install client
	actionCfg := &action.Configuration{
		Capabilities: chartutil.DefaultCapabilities,
	}
	for _, apiVersion := range cfg.APIVersions {
		actionCfg.Capabilities.APIVersions = append(actionCfg.Capabilities.APIVersions, apiVersion)
	}

	client := action.NewInstall(actionCfg)
	client.DryRun = true
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.DependencyUpdate = true
	client.Verify = cfg.Verify
	client.Keyring = cfg.Keyring
	client.ReleaseName = cfg.Metadata.Name
	client.Namespace = cfg.Metadata.Namespace
	client.IncludeCRDs = true

	// Load chart
	chartName := cfg.Chart
	chartPath := ""
	var chartRequested *chart.Chart
	if cfg.Repository == "" {
		if cfg.Chart != "." && !strings.HasPrefix(cfg.Chart, "./") {
			return fmt.Errorf("chart name must start with ./ if no repository specified")
		}
		chartPath, err = securePath(cfg.Chart, cfg.BaseDir, cfg.RootDir)
		if err != nil {
			return fmt.Errorf("no repository specified and invalid local chart path provided: %w", err)
		}
		chartName = filepath.Join(cfg.BaseDir, cfg.Chart)
		chartRequested, err = loader.Load(chartPath)
		if err != nil {
			return err
		}

		localCharts := make([]localChart, 0, 1)
		requiredRepos := map[string]struct{}{}
		err = collectLocalChartsAndRepos(chartRequested, chartPath, cfg, &localCharts, requiredRepos, 0)
		if err != nil {
			return err
		}

		// Write temporary repository configuration that includes all dependencies
		tmpRepos, err := tempRepositoriesWithDependencies(settings, getters, requiredRepos)
		if err != nil {
			return fmt.Errorf("init temp repositories.yaml: %w", err)
		}
		defer tmpRepos.Close()

		// Build local charts recursively
		needsReload, err := buildLocalCharts(localCharts, &cfg.LoadChartConfig, getters, settings)
		if err != nil {
			return fmt.Errorf("build/fetch dependencies: %w", err)
		}
		// Reload the chart with the updated Chart.lock file
		if needsReload {
			chartRequested, err = loader.Load(chartPath)
			if err != nil {
				return errors.Wrapf(err, "failed reloading chart %s after dependency download", chartName)
			}
		}
	} else {
		r, repos, err := useRepo(cfg.Repository, settings, getters)
		if err != nil {
			return err
		}
		defer repos.Close()
		client.Username = r.Username
		client.Password = r.Password
		client.CaFile = r.CAFile
		client.CertFile = r.CertFile
		client.KeyFile = r.KeyFile
		//tmpSettings := *settings
		//tmpSettings.RepositoryCache = filepath.Join(tmpSettings.RepositoryCache, "khelm", r.Name)
		/*chartPath, err := client.ChartPathOptions.LocateChart(cfg.Chart, settings)
		if err != nil {
			return err
		}*/
		chartPath, err = locateChart(&cfg.LoadChartConfig, repos, settings, getters)
		if err != nil {
			return err
		}
		chartRequested, err = loader.Load(chartPath)
		if err != nil {
			return err
		}
	}

	if err = checkIfInstallable(chartRequested); err != nil {
		return err
	}

	for _, chrt := range append(chartRequested.Dependencies(), chartRequested) {
		if chrt.Metadata.Deprecated {
			log.Printf("WARNING: Chart %q is deprecated", chrt.Name())
		}
	}

	log.Printf("Rendering chart %s %s at %s", chartRequested.Metadata.Name, chartRequested.Metadata.Version, chartPath)

	release, err := client.Run(chartRequested, vals)
	if err != nil {
		return fmt.Errorf("render chart: %w", err)
	}
	exclusions := Matchers(cfg.Exclude)
	manifestYAML, err := transform(release.Manifest, client.Namespace, exclusions)
	if err != nil {
		return err
	}
	for _, exclusion := range exclusions {
		if !exclusion.Matched {
			return errors.Errorf("exclusion selector did not match: %#v", exclusion.K8sObjectID)
		}
	}
	if manifestYAML == "" {
		return fmt.Errorf("chart %s: rendered chart manifest is empty - chart built? templates present?", chartName)
	}
	_, err = fmt.Fprintln(writer, manifestYAML)
	return err
}

// checkIfInstallable validates if a chart can be installed
//
// Application chart type is only installable
func checkIfInstallable(ch *chart.Chart) error {
	switch ch.Metadata.Type {
	case "", "application":
		return nil
	}
	return errors.Errorf("cannot install chart %q since it is of type %q", ch.Name(), ch.Metadata.Type)
}

type localChart struct {
	Chart               *chart.Chart
	Path                string
	DependsOnLocalChart bool
}

func collectLocalChartsAndRepos(chartRequested *chart.Chart, chartPath string, cfg *GeneratorConfig, localCharts *[]localChart, repos map[string]struct{}, depth int) error {
	if depth > 20 {
		return fmt.Errorf("collect local charts recursively: max depth of 20 reached - cyclic dependency?")
	}
	if req := chartRequested.Metadata.Dependencies; req != nil {
		dependsOnLocalChart := false
		for _, dep := range req {
			if strings.HasPrefix(dep.Repository, "file://") {
				dependsOnLocalChart = true
				depChartPath := strings.TrimPrefix(dep.Repository, "file://")
				depChartPath, err := securePath(depChartPath, cfg.BaseDir, cfg.RootDir)
				if err != nil {
					return fmt.Errorf("chart %s dependency %s repository: %w", chartRequested.Name(), dep.Name, err)
				}
				depChart, err := loader.LoadDir(depChartPath)
				if err != nil {
					return err
				}
				if err = collectLocalChartsAndRepos(depChart, depChartPath, cfg, localCharts, repos, depth+1); err != nil {
					return err
				}
			} else if strings.HasPrefix(dep.Repository, "https://") || strings.HasPrefix(dep.Repository, "http://") {
				repos[dep.Repository] = struct{}{}
			}
		}
		*localCharts = append(*localCharts, localChart{
			Chart:               chartRequested,
			Path:                chartPath,
			DependsOnLocalChart: dependsOnLocalChart,
		})
	}
	return nil
}

func buildLocalCharts(localCharts []localChart, cfg *LoadChartConfig, getters getter.Providers, settings *cli.EnvSettings) (needsReload bool, err error) {
	for _, ch := range localCharts {
		if err = action.CheckDependencies(ch.Chart, ch.Chart.Metadata.Dependencies); err != nil || ch.DependsOnLocalChart {
			needsReload = true
			if ch.DependsOnLocalChart {
				if err = os.RemoveAll(filepath.Join(ch.Path, "charts")); err != nil {
					return false, err
				}
			}
			log.Printf("Building/fetching chart %s %s dependencies", ch.Chart.Name(), ch.Chart.Metadata.Version)
			err = os.RemoveAll(filepath.Join(ch.Chart.ChartPath(), "tmpcharts"))
			if err != nil {
				return false, err
			}
			if err = buildChartDependencies(ch.Chart, ch.Path, cfg, getters, settings); err != nil {
				return false, fmt.Errorf("chart %s: %w", ch.Chart.Name(), err)
			}
		}
	}
	return needsReload, nil
}

func buildChartDependencies(chartRequested *chart.Chart, chartPath string, cfg *LoadChartConfig, getters getter.Providers, settings *cli.EnvSettings) error {
	man := &downloader.Manager{
		Out:              log.Writer(),
		ChartPath:        chartPath,
		Keyring:          cfg.Keyring,
		SkipUpdate:       true,
		Getters:          getters,
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
		Debug:            settings.Debug,
	}
	if cfg.Verify {
		man.Verify = downloader.VerifyAlways
	}
	// Downloads dependencies - respecting requirements.lock if present
	return man.Build()
}

func transform(manifest string, namespace string, excludes []*K8sObjectMatcher) (string, error) {
	obj, err := ParseObjects(bytes.NewReader([]byte(manifest)))
	if err != nil {
		return "", errors.Errorf("parse helm output manifest: %s, manifest: %q", err, manifest)
	}
	obj.ApplyDefaultNamespace(namespace)
	obj.Remove(excludes)
	return obj.Yaml(), nil
}
