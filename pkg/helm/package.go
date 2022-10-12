package helm

import (
	"context"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
)

func packageHelmChart(ctx context.Context, cfg *config.ChartConfig, destDir string, trustAnyRepo *bool, settings cli.EnvSettings, getters getter.Providers) (string, error) {
	// TODO: add unit test (there is an e2e/cli test for this though)
	_, err := buildAndLoadLocalChart(ctx, cfg, trustAnyRepo, settings, getters)
	if err != nil {
		return "", err
	}
	// See https://github.com/helm/helm/blob/v3.10.0/cmd/helm/package.go#L104
	client := action.NewPackage()
	client.Destination = destDir
	chartPath := absPath(cfg.Chart, cfg.BaseDir)
	tgzFile, err := client.Run(chartPath, map[string]interface{}{})
	if err != nil {
		return "", err
	}
	return tgzFile, nil
}
