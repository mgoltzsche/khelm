package helm

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/mgoltzsche/khelm/v2/internal/matcher"
	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/pkg/errors"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/release"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Render manifest from helm chart configuration (shorthand)
func (h *Helm) Render(ctx context.Context, req *config.ChartConfig) (r []*yaml.RNode, err error) {
	if errs := req.Validate(); len(errs) > 0 {
		return nil, errors.Errorf("invalid chart renderer config:\n * %s", strings.Join(errs, "\n * "))
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if req.BaseDir == "" {
		req.BaseDir = wd
	} else if !filepath.IsAbs(req.BaseDir) {
		req.BaseDir = filepath.Join(wd, req.BaseDir)
	}

	chartRequested, err := h.loadChart(ctx, req)
	if err != nil {
		return nil, errors.Wrapf(err, "load chart %s", req.Chart)
	}

	ch := make(chan struct{}, 1)
	go func() {
		r, err = renderChart(chartRequested, req, h.Getters)
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		return r, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// renderChart renders a manifest from the given chart and values.
// Derived from https://github.com/helm/helm/blob/v3.5.4/cmd/helm/template.go
func renderChart(chartRequested *chartv2.Chart, req *config.ChartConfig, getters getter.Providers) ([]*yaml.RNode, error) {
	log.Printf("Rendering chart %s %s with name %q and namespace %q", chartRequested.Metadata.Name, chartRequested.Metadata.Version, req.Name, req.Namespace)

	// Load values
	vals, err := loadValues(req, getters)
	if err != nil {
		return nil, err
	}

	// Run helm install client
	actionCfg := &action.Configuration{Capabilities: common.DefaultCapabilities}
	actionCfg.Capabilities.APIVersions = append(actionCfg.Capabilities.APIVersions, req.APIVersions...)
	if req.KubeVersion != "" {
		actionCfg.Capabilities.KubeVersion, err = parseKubeVersion(req.KubeVersion)
		if err != nil {
			return nil, err
		}
	}
	client := action.NewInstall(actionCfg)
	client.DryRunStrategy = action.DryRunClient
	client.Replace = true // Skip the name check
	client.DependencyUpdate = true
	client.Verify = req.Verify
	client.Keyring = req.Keyring
	client.ReleaseName = req.Name
	client.Namespace = req.Namespace
	client.IncludeCRDs = !req.ExcludeCRDs

	rel, err := client.Run(chartRequested, vals)
	if err != nil {
		return nil, errors.Wrapf(err, "render chart %s", chartRequested.Metadata.Name)
	}

	inclusions := matcher.Any()
	if len(req.Include) > 0 {
		inclusions = matcher.FromResourceSelectors(req.Include)
	}

	transformer := manifestTransformer{
		ForceNamespace: req.ForceNamespace,
		Includes:       inclusions,
		Excludes:       matcher.FromResourceSelectors(req.Exclude),
		NamespacedOnly: req.NamespacedOnly,
	}
	chartHookMatcher := matcher.NewChartHookMatcher(transformer.Excludes, !req.ExcludeHooks)
	transformer.Excludes = chartHookMatcher

	accessor, err := release.NewAccessor(rel)
	if err != nil {
		return nil, errors.Wrapf(err, "create release accessor for chart %s", chartRequested.Metadata.Name)
	}
	manifest := accessor.Manifest()
	for _, hook := range accessor.Hooks() {
		hookAccessor, err := release.NewHookAccessor(hook)
		if err != nil {
			return nil, errors.Wrapf(err, "create hook accessor for chart %s", chartRequested.Metadata.Name)
		}
		manifest += fmt.Sprintf("\n---\n%s", hookAccessor.Manifest())
	}

	transformed, err := transformer.TransformManifest(bytes.NewReader([]byte((manifest))))
	if err != nil {
		return nil, err
	}

	if err = transformer.Includes.RequireAllMatched(); err != nil {
		return nil, errors.Wrap(err, "resource inclusion")
	}
	if err = transformer.Excludes.RequireAllMatched(); err != nil {
		return nil, errors.Wrap(err, "resource exclusion")
	}
	if len(transformed) == 0 {
		return nil, errors.Errorf("chart %s output is empty", chartRequested.Metadata.Name)
	}
	if hooks := chartHookMatcher.FoundHooks(); !req.ExcludeHooks && len(hooks) > 0 {
		log.Printf("WARNING: Chart output contains the following hooks: %s", strings.Join(hooks, ", "))
	}
	return transformed, nil
}

func parseKubeVersion(version string) (kv common.KubeVersion, err error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return kv, errors.Wrapf(err, "invalid kubeVersion %q provided", version)
	}
	if v.Prerelease() != "" || v.Metadata() != "" {
		return kv, errors.Errorf("invalid kubeVersion %q provided: unexpected version suffix", version)
	}
	return common.KubeVersion{
		Version: fmt.Sprintf("v%d.%d.%d", v.Major(), v.Minor(), v.Patch()),
		Major:   strconv.FormatUint(v.Major(), 10),
		Minor:   strconv.FormatUint(v.Minor(), 10),
	}, nil
}
