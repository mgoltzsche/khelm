package helm

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type manifestTransformer struct {
	Namespace      string
	Excludes       ResourceMatchers
	NamespacedOnly bool
	OutputPath     string
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

		if o.IsNilOrEmpty() {
			continue
		}

		meta, err := o.GetMeta()
		if err != nil {
			break
		}

		// Filter excluded resources
		if t.Excludes.MatchAny(&meta) {
			continue
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
			"but the following cluster-scoped (or unknown) resources have been found:\n * %s\nPlease exclude cluster-scoped resources or enable their usage", strings.Join(clusterScopedResources, "\n * "))
	}
	return
}

func (t *manifestTransformer) applyNamespace(o *yaml.RNode, clusterScopedResources *[]string) error {
	meta, err := o.GetMeta()
	if err != nil {
		return nil
	}
	namespaced, knownKind := openapi.IsNamespaceScoped(meta.TypeMeta)
	if t.Namespace != "" && (namespaced || !knownKind) {
		err = o.PipeE(yaml.LookupCreate(
			yaml.ScalarNode, "metadata", "namespace"),
			yaml.FieldSetter{StringValue: t.Namespace})
		if err != nil {
			return err
		}
	} else if t.NamespacedOnly && (!namespaced || !knownKind) && meta.Namespace == "" {
		resID := fmt.Sprintf("apiVersion: %s, kind: %s, name: %s", meta.APIVersion, meta.Kind, meta.Name)
		*clusterScopedResources = append(*clusterScopedResources, resID)
	}
	return nil
}
