package helm

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// ResourceSelector specifies a Kubernetes resource selector
type ResourceSelector struct {
	APIVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
	Name       string `yaml:"name,omitempty"`
}

// Match returns true if all non-empty fields match the ones in the provided object
func (id *ResourceSelector) Match(o *yaml.ResourceMeta) bool {
	return (id.APIVersion == "" || id.APIVersion == o.APIVersion) &&
		(id.Kind == "" || id.Kind == o.Kind) &&
		(id.Namespace == "" || id.Namespace == o.Namespace) &&
		(id.Name == "" || id.Name == o.Name)
}

// ResourceMatchers is a group of matchers
type ResourceMatchers []*resourceMatcher

// ResourceMatcher matches
type resourceMatcher struct {
	ResourceSelector
	Matched bool
}

// RequireAllMatched returns an error if any matcher did not match
func (m ResourceMatchers) RequireAllMatched() error {
	for _, e := range m {
		if !e.Matched {
			return errors.Errorf("did not match: %#v", e.ResourceSelector)
		}
	}
	return nil
}

// MatchAny returns true if any matches matches the given object
func (m ResourceMatchers) MatchAny(o *yaml.ResourceMeta) bool {
	for _, e := range m {
		if e.ResourceSelector.Match(o) {
			e.Matched = true
			return true
		}
	}
	return false
}

// Matchers creates matchers from the provided selectors
func Matchers(selectors []ResourceSelector) ResourceMatchers {
	matchers := make([]*resourceMatcher, len(selectors))
	for i, selector := range selectors {
		matchers[i] = &resourceMatcher{selector, false}
	}
	return matchers
}
