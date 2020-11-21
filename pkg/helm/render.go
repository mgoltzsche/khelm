package helm

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var whitespaceRegex = regexp.MustCompile(`^\s*$`)

// Render manifest from helm chart configuration (shorthand)
func (h *Helm) Render(ctx context.Context, req ChartConfig) (r []*yaml.RNode, err error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if req.BaseDir == "" {
		req.BaseDir = wd
	} else if !filepath.IsAbs(req.BaseDir) {
		req.BaseDir = filepath.Join(wd, req.BaseDir)
	}

	chartRequested, err := h.loadChart(ctx, &req)
	if err != nil {
		return nil, err
	}

	log.Printf("Rendering chart %s %s with name %q and namespace %q", chartRequested.Metadata.Name, chartRequested.Metadata.Version, req.Name, req.Namespace)

	return renderChart(chartRequested, &req, h.Getters)
}

// renderChart renders a manifest from the given chart and values
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/template.go
func renderChart(chrt *chart.Chart, c *ChartConfig, getters getter.Providers) (r []*yaml.RNode, err error) {
	namespace := c.Namespace
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Name:      c.Name,
			Namespace: namespace,
		},
		KubeVersion: c.KubeVersion,
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
		return nil, errors.Errorf("chart %s does not contain any manifests", chrt.Metadata.Name)
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
