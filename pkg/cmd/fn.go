package cmd

import (
	"os"
	"strconv"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/mgoltzsche/helmr/pkg/output"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	apiVersionConfigKubernetesIO = "config.kubernetes.io"
	annotationIndex              = apiVersionConfigKubernetesIO + "/index"
	annotationPath               = apiVersionConfigKubernetesIO + "/path"
)

func kptFnCommand(cfg helm.Config) *cobra.Command {
	req := helm.NewChartConfig()
	resourceList := &framework.ResourceList{FunctionConfig: &kptFunctionConfigMap{}}
	cmd := framework.Command(resourceList, func() (err error) {
		rendered, err := render(cfg, req)
		if err != nil {
			return err
		}
		addKptAnnotations(rendered, "output")
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

type kptFunctionConfigMap struct {
	Data map[string]string
}

func addKptAnnotations(resources []*yaml.RNode, outputBasePath string) {
	for i, o := range resources {
		meta, err := o.GetMeta()
		if err != nil {
			continue
		}

		// Set kpt order and path annotations
		outPath := output.ResourcePath(meta, outputBasePath)
		lookupAnnotations := yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField, yaml.AnnotationsField)
		err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationIndex, StringValue: strconv.Itoa(i)})
		if err != nil {
			continue
		}
		err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationPath, StringValue: outPath})
		if err != nil {
			continue
		}
	}
}
