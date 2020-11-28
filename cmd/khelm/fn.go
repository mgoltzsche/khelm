package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/mgoltzsche/khelm/internal/output"
	"github.com/mgoltzsche/khelm/pkg/helm"
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
	req := helm.NewChartConfig()
	fnCfg := kptFnConfigMap{Data: kptFnConfig{ChartConfig: req}}
	resourceList := &framework.ResourceList{FunctionConfig: &fnCfg}
	cmd := framework.Command(resourceList, func() (err error) {
		h.Settings.Debug = h.Settings.Debug || fnCfg.Data.Debug
		rendered, err := render(h, req)
		if err != nil {
			return err
		}

		outputPath := fnCfg.Data.OutputPath
		if outputPath == "" {
			outputPath = defaultOutputPath
		}

		if output.IsDirectory(outputPath) {
			kustomization, err := generateKustomization(rendered)
			if err != nil {
				return errors.Wrap(err, "generate kustomization.yaml")
			}
			rendered = append(rendered, kustomization)
		}

		addKptAnnotations(rendered, outputPath)
		resourceList.Items = filterByOutputPath(resourceList.Items, outputPath)
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
	*helm.ChartConfig `yaml:",inline"`
	OutputPath        string `yaml:"outputPath,omitempty"`
	Debug             bool   `yaml:"debug,omitempty"`
}

func filterByOutputPath(resources []*yaml.RNode, outputPath string) []*yaml.RNode {
	r := make([]*yaml.RNode, 0, len(resources))
	for _, o := range resources {
		meta, err := o.GetMeta()
		if err != nil || meta.Annotations == nil || !isGeneratedOutputPath(meta.Annotations[annotationPath], outputPath) {
			r = append(r, o)
		}
	}
	return r
}

func isGeneratedOutputPath(path, outputPath string) bool {
	return path == outputPath || output.IsDirectory(outputPath) && strings.HasPrefix(path, outputPath)
}

func addKptAnnotations(resources []*yaml.RNode, outputPath string) []string {
	outPath := outputPath
	outPaths := make([]string, 0, len(resources))
	for i, o := range resources {
		meta, err := o.GetMeta()
		if err != nil {
			continue
		}

		// Set kpt order and path annotations
		if output.IsDirectory(outPath) {
			outPath = output.ResourcePath(meta, outputPath)
		}
		lookupAnnotations := yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField, yaml.AnnotationsField)
		_ = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationIndex, StringValue: strconv.Itoa(i)})
		_ = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationPath, StringValue: outPath})
		outPaths = append(outPaths, outPath)
	}
	return outPaths
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
