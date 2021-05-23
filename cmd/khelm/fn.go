package main

import (
	"encoding/json"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/mgoltzsche/khelm/v2/internal/matcher"
	"github.com/mgoltzsche/khelm/v2/internal/output"
	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/helm"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	apiVersionConfigKubernetesIO = "config.kubernetes.io"
	annotationIndex              = apiVersionConfigKubernetesIO + "/index"
	annotationPath               = apiVersionConfigKubernetesIO + "/path"
	defaultOutputPath            = "generated-manifest.yaml"
)

func kptFnCommand(h *helm.Helm) *cobra.Command {
	req := config.NewChartConfig()
	fnCfg := kptFnConfigMap{Data: kptFnConfig{ChartConfig: req}}
	resourceList := &framework.ResourceList{FunctionConfig: &fnCfg}
	cmd := framework.Command(resourceList, func() (err error) {
		outputPath := fnCfg.Data.OutputPath
		if outputPath == "" {
			outputPath = defaultOutputPath
		}
		outputPaths := make([]string, len(fnCfg.Data.OutputPathMapping)+1)
		outputPaths[0] = outputPath
		for i, m := range fnCfg.Data.OutputPathMapping {
			outputPaths[i+1] = m.OutputPath
			if m.OutputPath == "" {
				return errors.Errorf("no outputPath specified for outputMapping[%d]", i)
			}
			if len(m.ResourceSelectors) == 0 {
				return errors.Errorf("no selectors specified for outputMapping[%d] -> %q", i, m.OutputPath)
			}
		}

		// Template the helm chart
		h.Settings.Debug = h.Settings.Debug || fnCfg.Data.Debug
		rendered, err := render(h, req)
		if err != nil {
			return err
		}

		// Apply output path mappings and annotate resources
		kustomizationDirs, err := mapOutputPaths(rendered, fnCfg.Data.OutputPathMapping, outputPath, h.Settings.Debug)
		if err != nil {
			return err
		}

		// Generate kustomizations
		dirs := make([]string, 0, len(kustomizationDirs))
		for dir := range kustomizationDirs {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		kustomizationResources := make([]*yaml.RNode, len(dirs))
		for i, dir := range dirs {
			resources := kustomizationDirs[dir]
			kustomizationPath := path.Join(dir, "kustomization.yaml")
			kustomization, err := generateKustomization(resources)
			if err != nil {
				return errors.Wrapf(err, "generate %s", kustomizationPath)
			}
			err = setKptAnnotations(kustomization, kustomizationPath, 0, h.Settings.Debug)
			if err != nil {
				return errors.Wrap(err, "set kpt annotations on kustomization")
			}
			kustomizationResources[i] = kustomization
		}
		rendered = append(kustomizationResources, rendered...)

		// Apply output
		resourceList.Items = filterByOutputPath(resourceList.Items, outputPaths)
		resourceList.Items = append(resourceList.Items, rendered...)
		return nil
	})
	return cmd
}

func generateKustomization(resources []*yaml.RNode) (*yaml.RNode, error) {
	resourcePaths := resourceNames(resources, "")
	m := map[string]interface{}{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  resourcePaths,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return yaml.ConvertJSONToYamlNode(string(b))
}

type kptFnConfigMap struct {
	Data kptFnConfig `yaml:"data"`
}

type kptFnConfig struct {
	*config.ChartConfig `yaml:",inline"`
	OutputPath          string               `yaml:"outputPath,omitempty"`
	OutputPathMapping   []kptFnOutputMapping `yaml:"outputPathMapping,omitempty"`
	Debug               bool                 `yaml:"debug,omitempty"`
}

type kptFnOutputMapping struct {
	ResourceSelectors []config.ResourceSelector `yaml:"selectors,omitempty"`
	OutputPath        string                    `yaml:"outputPath"`
}

func filterByOutputPath(resources []*yaml.RNode, outputPaths []string) []*yaml.RNode {
	r := make([]*yaml.RNode, 0, len(resources))
	for _, o := range resources {
		meta, err := o.GetMeta()
		if err != nil || meta.Annotations == nil || !isGeneratedOutputPath(meta.Annotations[annotationPath], outputPaths) {
			r = append(r, o)
		}
	}
	return r
}

func isGeneratedOutputPath(path string, outputPaths []string) bool {
	for _, p := range outputPaths {
		if path == p || output.IsDirectory(p) && strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func mapOutputPaths(resources []*yaml.RNode, outputMappings []kptFnOutputMapping, defaultOutputPath string, debug bool) (map[string][]*yaml.RNode, error) {
	matchers := make([]matcher.ResourceMatchers, len(outputMappings))
	for i, m := range outputMappings {
		matchers[i] = matcher.FromResourceSelectors(m.ResourceSelectors)
	}
	kustomizationDirs := map[string][]*yaml.RNode{}
	for i, o := range resources {
		meta, err := o.GetMeta()
		if err != nil {
			continue
		}

		outPath := defaultOutputPath
		for i, m := range matchers {
			if m.Match(&meta) {
				outPath = outputMappings[i].OutputPath
				break
			}
		}

		// Set kpt order and path annotations
		if output.IsDirectory(outPath) {
			kustomizationDirs[outPath] = append(kustomizationDirs[outPath], o)
			outPath = output.ResourcePath(meta, outPath)
		}
		err = setKptAnnotations(o, outPath, i, debug)
		if err != nil {
			return nil, errors.Wrapf(err, "set annotations on %s/%s", meta.Kind, meta.Name)
		}
	}

	for _, m := range matchers {
		err := m.RequireAllMatched()
		if err != nil {
			return nil, errors.Wrap(err, "outputMapping")
		}
	}

	return kustomizationDirs, nil
}

func setKptAnnotations(o *yaml.RNode, path string, index int, debug bool) error {
	if debug {
		m, err := o.GetMeta()
		if err != nil {
			return err
		}
		log.Printf("Mapping %s %s to path %s", m.Kind, m.Name, path)
	}
	// Remove annotations field if empty.
	// This is required because LookupCreate() doesn't create the MappingNode if it exists but is empty (#13).
	err := o.PipeE(yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField), yaml.FieldClearer{Name: yaml.AnnotationsField, IfEmpty: true})
	if err != nil {
		return err
	}
	// Add path annotations
	lookupAnnotations := yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField, yaml.AnnotationsField)
	err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationIndex, StringValue: strconv.Itoa(index)})
	if err != nil {
		return err
	}
	err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationPath, StringValue: path})
	if err != nil {
		return err
	}
	return nil
}

func resourceNames(resources []*yaml.RNode, outputBasePath string) []string {
	outPaths := make([]string, 0, len(resources))
	for _, o := range resources {
		if meta, err := o.GetMeta(); err == nil {
			outPaths = append(outPaths, output.ResourcePath(meta, outputBasePath))
		}
	}
	return outPaths
}
