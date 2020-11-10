package helm

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/repo"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var (
	whitespaceRegex    = regexp.MustCompile(`^\s*$`)
	defaultKubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)
	Debug, _           = strconv.ParseBool(os.Getenv("HELM_DEBUG"))
)

// Render manifest from helm chart configuration (shorthand)
func Render(ctx context.Context, cfg *ChartConfig) (r []*yaml.RNode, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = wd
	} else if !filepath.IsAbs(cfg.BaseDir) {
		cfg.BaseDir = filepath.Join(wd, cfg.BaseDir)
	}
	helmHome := os.Getenv("HELM_HOME")
	if helmHome == "" {
		helmHome = environment.DefaultHelmHome
	}
	settings := environment.EnvSettings{
		Home:  helmpath.Home(helmHome),
		Debug: Debug,
	}
	getters := getter.All(settings)

	if err = initializeHelmHome(settings.Home); err != nil {
		return nil, err
	}

	chartRequested, err := loadChart(ctx, cfg, &settings, getters)
	if err != nil {
		return nil, err
	}

	log.Printf("Rendering chart %s %s with release name %q and namespace %q", chartRequested.Metadata.Name, chartRequested.Metadata.Version, cfg.ReleaseName, cfg.Namespace)

	return renderChart(chartRequested, cfg, getters)
}

// Initialize initialize the helm home directory.
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/installer/init.go
func initializeHelmHome(home helmpath.Home) (err error) {
	// Create directories
	for _, dir := range []string{
		home.String(),
		home.Repository(),
		home.Cache(),
		home.LocalRepository(),
		home.Plugins(),
		home.Starters(),
		home.Archive(),
	} {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return errors.WithStack(err)
		}
	}

	// Create repo file
	if _, err = os.Stat(home.RepositoryFile()); err != nil && os.IsNotExist(err) {
		log.Printf("Initializing empty %s", home.RepositoryFile())
		f := repo.NewRepoFile()
		err = f.WriteFile(home.RepositoryFile(), 0644)
	}
	return errors.WithStack(err)
}

// renderChart renders a manifest from the given chart and values
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/template.go
func renderChart(chrt *chart.Chart, c *ChartConfig, getters getter.Providers) (r []*yaml.RNode, err error) {
	namespace := c.Namespace
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Name:      c.ReleaseName,
			Namespace: namespace,
		},
		KubeVersion: defaultKubeVersion,
	}
	if len(c.APIVersions) > 0 {
		renderOpts.APIVersions = append(c.APIVersions, "v1")
	}

	rawVals, err := vals(chrt, c.ValueFiles, c.Values, c.BaseDir, getters, "", "", "")
	if err != nil {
		return nil, errors.Wrap(err, "load values")
	}
	config := &chart.Config{Raw: string(rawVals), Values: map[string]*chart.Value{}}

	renderedTemplates, err := renderutil.Render(chrt, config, renderOpts)
	if err != nil {
		return nil, errors.Wrap(err, "render chart")
	}

	listManifests := manifest.SplitManifests(renderedTemplates)

	if len(listManifests) == 0 {
		return nil, errors.Errorf("chart %s does not contain any manifests - chart built? templates present?", chrt.Metadata.Name)
	}

	transformer := manifestTransformer{
		Namespace:          namespace,
		Excludes:           Matchers(c.Exclude),
		AllowClusterScoped: c.ClusterScoped,
		OutputPath:         "output",
	}

	r = make([]*yaml.RNode, 0, len(listManifests))
	for _, m := range sortByKind(listManifests) {
		b := filepath.Base(m.Name)
		if b == "NOTES.txt" || strings.HasPrefix(b, "_") || whitespaceRegex.MatchString(m.Content) {
			continue
		}
		transformed, err := transformer.TransformManifest(bytes.NewReader([]byte(m.Content)))
		if err != nil {
			return nil, errors.WithMessage(err, filepath.Base(m.Name))
		}
		r = append(r, transformed...)
	}

	if err = transformer.Excludes.RequireAllMatched(); err != nil {
		return nil, errors.Wrap(err, "resource exclusion selector")
	}

	return
}
