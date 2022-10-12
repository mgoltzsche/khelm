package helm

import (
	"context"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/getter/git"
	"helm.sh/helm/v3/pkg/cli"
	helmgetter "helm.sh/helm/v3/pkg/getter"
)

func getters(settings *cli.EnvSettings, trustAnyRepo **bool) helmgetter.Providers {
	g := helmgetter.All(settings)
	g = append(g, helmgetter.Provider{
		Schemes: []string{"git+https", "git+ssh"},
		New: git.New(settings, func(ctx context.Context, chartDir, repoDir string) (string, error) {
			return packageHelmChart(ctx, &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: chartDir,
				},
				BaseDir: repoDir,
			}, chartDir, *trustAnyRepo, *settings, g)
		}),
	})
	return g
}
