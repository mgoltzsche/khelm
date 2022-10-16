package helm

import (
	"context"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/getter/git"
	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"helm.sh/helm/v3/pkg/cli"
	helmgetter "helm.sh/helm/v3/pkg/getter"
)

func getters(settings *cli.EnvSettings, reposFn func() (repositories.Interface, error)) helmgetter.Providers {
	g := helmgetter.All(settings)
	g = append(g, helmgetter.Provider{
		Schemes: []string{"git+https", "git+ssh"},
		New: git.New(settings, reposFn, func(ctx context.Context, chartDir, repoDir string) (string, error) {
			repos, err := reposFn()
			if err != nil {
				return "", err
			}
			return packageHelmChart(ctx, &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: chartDir,
				},
				BaseDir: repoDir,
			}, chartDir, repos, *settings, g)
		}),
	})
	return g
}
