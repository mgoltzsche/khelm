package cmd

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/mgoltzsche/helmr/pkg/internal/output"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	apiVersionConfigKubernetesIO = "config.kubernetes.io"
	annotationIndex              = apiVersionConfigKubernetesIO + "/index"
	annotationPath               = apiVersionConfigKubernetesIO + "/path"
)

func kptFnCommand(cfg *helm.Config) *cobra.Command {
	req := helm.NewChartConfig()
	fnCfg := kptFnConfigMap{Data: kptFnConfig{&req, "", false, cfg.Debug}}
	resourceList := &framework.ResourceList{FunctionConfig: &fnCfg}
	cmd := framework.Command(resourceList, func() (err error) {
		cfg.Debug = cfg.Debug || fnCfg.Data.Debug
		rendered, err := render(*cfg, req)
		if err != nil {
			return err
		}

		outputPath := fnCfg.Data.OutputPath
		if outputPath == "" {
			outputPath = "chart-output"
		}

		if fnCfg.Data.OutputKustomization {
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
	cmd.Example = usageExample
	cmd.Use = os.Args[0]
	cmd.Short = "helmr chart renderer"
	cmd.Long = `helmr is a helm chart templating CLI, kustomize plugin and kpt function.

In opposite to the original helm CLI helmr supports:
* usage of any repositories without registering them in repositories.yaml
* building local charts recursively when templating`
	return &cmd
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
	*helm.ChartConfig   `yaml:",inline"`
	OutputPath          string `yaml:"outputPath,omitempty"`
	OutputKustomization bool   `yaml:"outputKustomization,omitempty"`
	Debug               bool   `yaml:"debug,omitempty"`
}

func filterByOutputPath(resources []*yaml.RNode, outputBasePath string) []*yaml.RNode {
	outputBasePath += "/"
	r := make([]*yaml.RNode, 0, len(resources))
	for _, o := range resources {
		meta, err := o.GetMeta()
		if err != nil || meta.Annotations == nil || !strings.HasPrefix(meta.Annotations[annotationPath], outputBasePath) {
			r = append(r, o)
		}
	}
	return r
}

func addKptAnnotations(resources []*yaml.RNode, outputBasePath string) []string {
	outPaths := make([]string, 0, len(resources))
	for i, o := range resources {
		meta, err := o.GetMeta()
		if err != nil {
			continue
		}

		// Set kpt order and path annotations
		outPath := output.ResourcePath(meta, outputBasePath)
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
