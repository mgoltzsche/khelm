package helm

import (
	"context"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
)

type PackageOptions struct {
	ChartDir string
	BaseDir  string
	DestDir  string
}

// Package builds and packages a local Helm chart.
// Returns the tgz file path.
func (h *Helm) Package(ctx context.Context, chartDir, baseDir, destDir string) (string, error) {
	repos, err := h.Repositories()
	if err != nil {
		return "", err
	}
	cfg := config.ChartConfig{
		LoaderConfig: config.LoaderConfig{
			Chart: chartDir,
		},
		BaseDir: baseDir,
	}
	return packageHelmChart(ctx, &cfg, repos, h.Settings, h.Getters)
}

func packageHelmChart(ctx context.Context, cfg *config.ChartConfig, repos repositories.Interface, settings cli.EnvSettings, getters getter.Providers) (string, error) {
	// TODO: add unit test (there is an e2e/cli test for this though)
	_, err := buildAndLoadLocalChart(ctx, cfg, repos, settings, getters)
	if err != nil {
		return "", err
	}
	// See https://github.com/helm/helm/blob/v3.10.0/cmd/helm/package.go#L104
	client := action.NewPackage()
	client.Destination = cfg.BaseDir
	chartPath := absPath(cfg.Chart, cfg.BaseDir)
	tgzFile, err := client.Run(chartPath, map[string]interface{}{})
	if err != nil {
		return "", err
	}
	return tgzFile, nil
}
