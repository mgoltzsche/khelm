package helm

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	apiVersionConfigKubernetesIO = "config.kubernetes.io"
	annotationIndex              = apiVersionConfigKubernetesIO + "/index"
	annotationPath               = apiVersionConfigKubernetesIO + "/path"
)

type manifestTransformer struct {
	Namespace          string
	Excludes           ResourceMatchers
	AllowClusterScoped bool
	OutputPath         string
}

func (t *manifestTransformer) TransformManifest(manifest io.Reader) (r []*yaml.RNode, err error) {
	clusterScopedResources := []string{}
	d := yaml.NewDecoder(manifest)
	for {
		v := yaml.Node{}
		o := yaml.NewRNode(&v)
		err = d.Decode(&v)
		if err != nil {
			break
		}

		meta, err := o.GetMeta()
		if err != nil {
			break
		}

		// Filter excluded resources
		if t.Excludes.MatchAny(&meta) {
			continue
		}

		// Set kpt order and path annotations
		outPath := path.Join(t.OutputPath, fmt.Sprintf("%s-%s.yaml", strings.ToLower(meta.Kind), meta.Name))
		lookupAnnotations := yaml.LookupCreate(yaml.MappingNode, yaml.MetadataField, yaml.AnnotationsField)
		err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationIndex, StringValue: strconv.Itoa(len(r))})
		if err != nil {
			break
		}
		err = o.PipeE(lookupAnnotations, yaml.FieldSetter{Name: annotationPath, StringValue: outPath})
		if err != nil {
			break
		}

		// Set namespace
		err = t.applyNamespace(o, &clusterScopedResources)
		if err != nil {
			break
		}

		r = append(r, o)
	}
	if err == io.EOF {
		err = nil
	} else if err != nil {
		return nil, errors.Wrap(err, "process helm output")
	}
	if len(clusterScopedResources) > 0 {
		return nil, errors.Errorf("manifests should only include namespace-scoped resources "+
			"but the following cluster-scoped resources have been found:\n  %s\nexclude cluster-scoped resources or enable their usage", strings.Join(clusterScopedResources, "\n  "))
	}
	return
}

func (t *manifestTransformer) applyNamespace(o *yaml.RNode, clusterScopedResources *[]string) error {
	meta, err := o.GetMeta()
	if err != nil {
		return nil
	}
	namespaced, knownType := openapi.IsNamespaceScoped(meta.TypeMeta)
	if !knownType {
		namespaced = true
	}
	if namespaced {
		if t.Namespace != "" {
			err = o.PipeE(yaml.LookupCreate(
				yaml.ScalarNode, "metadata", "namespace"),
				yaml.FieldSetter{StringValue: t.Namespace})
			if err != nil {
				return err
			}
		}
	} else {
		if !t.AllowClusterScoped {
			resID := fmt.Sprintf("apiVersion: %s, kind: %s, name: %s", meta.APIVersion, meta.Kind, meta.Name)
			*clusterScopedResources = append(*clusterScopedResources, resID)
		}
	}
	return nil
}
