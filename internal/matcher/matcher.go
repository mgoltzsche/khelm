package matcher

import (
	"fmt"
	"strings"

	"github.com/mgoltzsche/khelm/pkg/config"
	"github.com/pkg/errors"
)

// KubernetesResourceMeta represents a kubernetes resource's meta data
type KubernetesResourceMeta interface {
	GetAPIVersion() string
	GetKind() string
	GetNamespace() string
	GetName() string
}

// ResourceMatchers is a group of matchers
type ResourceMatchers interface {
	Match(o KubernetesResourceMeta) bool
	RequireAllMatched() error
}

// Any returns a resource matches that matches any resource
func Any() ResourceMatchers {
	return &matchAny{}
}

type matchAny struct{}

func (m *matchAny) RequireAllMatched() error          { return nil }
func (m *matchAny) Match(KubernetesResourceMeta) bool { return true }

type resourceMatchers []*resourceMatcher

type resourceMatcher struct {
	config.ResourceSelector
	Matched bool
}

// RequireAllMatched returns an error if any matcher did not match
func (m resourceMatchers) RequireAllMatched() error {
	var errs []string
	for _, e := range m {
		if !e.Matched {
			errs = append(errs, fmt.Sprintf("%#v", e.ResourceSelector))
		}
	}
	if len(errs) > 0 {
		return errors.Errorf("selectors did not match:\n * %s", strings.Join(errs, "\n * "))
	}
	return nil
}

// MatchAny returns true if any matches matches the given object
func (m resourceMatchers) Match(o KubernetesResourceMeta) bool {
	for _, e := range m {
		if e.ResourceSelector.Match(o) {
			e.Matched = true
			return true
		}
	}
	return false
}

// FromResourceSelectors creates matchers from the provided selectors
func FromResourceSelectors(selectors []config.ResourceSelector) ResourceMatchers {
	matchers := make([]*resourceMatcher, len(selectors))
	for i, selector := range selectors {
		matchers[i] = &resourceMatcher{selector, false}
	}
	return resourceMatchers(matchers)
}
