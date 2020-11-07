package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
)

var (
	whitespaceRegex    = regexp.MustCompile(`^\s*$`)
	defaultKubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)
)

// Render manifest from helm chart configuration (shorthand)
func Render(ctx context.Context, cfg *ChartConfig, writer io.Writer) (err error) {
	if cfg.RootDir == "" {
		cfg.RootDir = string(filepath.Separator)
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir, err = os.Getwd()
		if err != nil {
			return errors.Wrap(err, "base dir not provided, cannot derive it from working dir: %w")
		}
	}
	helmHome := os.Getenv("HELM_HOME")
	if helmHome == "" {
		helmHome = environment.DefaultHelmHome
	}
	debug, _ := strconv.ParseBool(os.Getenv("HELM_DEBUG"))
	settings := environment.EnvSettings{
		Home:  helmpath.Home(helmHome),
		Debug: debug,
	}
	getters := getter.All(settings)

	if err = initializeHelmHome(settings.Home); err != nil {
		return err
	}

	chartRequested, err := loadChart(ctx, cfg, &settings, getters)
	if err != nil {
		return err
	}

	log.Printf("Rendering chart %s %s", chartRequested.Metadata.Name, chartRequested.Metadata.Version)

	return renderChart(chartRequested, cfg, getters, writer)
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
			return err
		}
	}

	// Create repo file
	if _, err = os.Stat(home.RepositoryFile()); err != nil && os.IsNotExist(err) {
		log.Printf("Initializing empty %s", home.RepositoryFile())
		f := repo.NewRepoFile()
		return f.WriteFile(home.RepositoryFile(), 0644)
	}
	return err
}

// renderChart renders a manifest from the given chart and values
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/template.go
func renderChart(chrt *chart.Chart, c *ChartConfig, getters getter.Providers, writer io.Writer) (err error) {
	namespace := c.Namespace
	if namespace == "" {
		namespace = "default" // avoids kustomize panic due to missing namespace
	}
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
	log.Printf("Rendering chart with name %q, namespace: %q\n", c.ReleaseName, namespace)

	rawVals, err := vals(chrt, c.ValueFiles, c.Values, c.RootDir, c.BaseDir, getters, "", "", "")
	if err != nil {
		return errors.Wrap(err, "load values")
	}
	config := &chart.Config{Raw: string(rawVals), Values: map[string]*chart.Value{}}

	renderedTemplates, err := renderutil.Render(chrt, config, renderOpts)
	if err != nil {
		return errors.Wrap(err, "render chart")
	}

	listManifests := manifest.SplitManifests(renderedTemplates)
	exclusions := Matchers(c.Exclude)

	if len(listManifests) == 0 {
		return errors.Errorf("chart %s does not contain any manifests - chart built? templates present?", chrt.Metadata.Name)
	}

	for _, m := range sortByKind(listManifests) {
		b := filepath.Base(m.Name)
		if b == "NOTES.txt" || strings.HasPrefix(b, "_") || whitespaceRegex.MatchString(m.Content) {
			continue
		}
		if err = transform(&m, namespace, exclusions); err != nil {
			return errors.WithMessage(err, filepath.Base(m.Name))
		}
		fmt.Fprintf(writer, "---\n# Source: %s\n", m.Name)
		fmt.Fprintln(writer, m.Content)
	}

	for _, exclusion := range exclusions {
		if !exclusion.Matched {
			return errors.Errorf("exclusion selector did not match: %#v", exclusion.K8sObjectID)
		}
	}

	return
}

func transform(m *manifest.Manifest, namespace string, excludes []*K8sObjectMatcher) error {
	obj, err := ParseObjects(bytes.NewReader([]byte(m.Content)))
	if err != nil {
		return errors.Errorf("%s: %q", err, m.Content)
	}
	obj.ApplyDefaultNamespace(namespace)
	obj.Remove(excludes)
	m.Content = obj.Yaml()
	return nil
}
