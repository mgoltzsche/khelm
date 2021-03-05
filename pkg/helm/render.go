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
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/getter"
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
		return nil, err
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

// renderChart renders a manifest from the given chart and values
func renderChart(chartRequested *chart.Chart, req *config.ChartConfig, getters getter.Providers) ([]*yaml.RNode, error) {
	log.Printf("Rendering chart %s %s with name %q and namespace %q", chartRequested.Metadata.Name, chartRequested.Metadata.Version, req.Name, req.Namespace)

	// Load values
	vals, err := loadValues(req, getters)
	if err != nil {
		return nil, err
	}

	// Run helm install client
	actionCfg := &action.Configuration{Capabilities: chartutil.DefaultCapabilities}
	actionCfg.Capabilities.APIVersions = append(actionCfg.Capabilities.APIVersions, req.APIVersions...)
	if req.KubeVersion != "" {
		actionCfg.Capabilities.KubeVersion, err = parseKubeVersion(req.KubeVersion)
		if err != nil {
			return nil, err
		}
	}
	client := action.NewInstall(actionCfg)
	client.DryRun = true
	client.Replace = true // Skip the name check
	client.ClientOnly = true
	client.DependencyUpdate = true
	client.Verify = req.Verify
	client.Keyring = req.Keyring
	client.ReleaseName = req.Name
	client.Namespace = req.Namespace
	client.IncludeCRDs = !req.ExcludeCRDs

	release, err := client.Run(chartRequested, vals)
	if err != nil {
		return nil, errors.Wrap(err, "render chart")
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
		OutputPath:     "khelm-output",
	}

	transformed, err := transformer.TransformManifest(bytes.NewReader([]byte(release.Manifest)))
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
	return transformed, nil
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

func parseKubeVersion(version string) (kv chartutil.KubeVersion, err error) {
	v, err := semver.NewVersion(version)
	if err != nil {
		return kv, errors.Wrapf(err, "invalid kubeVersion %q provided", version)
	}
	if v.Prerelease() != "" || v.Metadata() != "" {
		return kv, errors.Errorf("invalid kubeVersion %q provided: unexpected version suffix", version)
	}
	return chartutil.KubeVersion{
		Version: fmt.Sprintf("v%d.%d.%d", v.Major(), v.Minor(), v.Patch()),
		Major:   strconv.FormatInt(int64(v.Major()), 10),
		Minor:   strconv.FormatInt(int64(v.Minor()), 10),
	}, nil
}
