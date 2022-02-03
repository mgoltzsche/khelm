package matcher

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const annotationHelmHook = "helm.sh/hook"

// ResourceMatchers is a group of matchers
type ResourceMatchers interface {
	Match(o *yaml.ResourceMeta) bool
	RequireAllMatched() error
}

// Any returns a resource matches that matches any resource
func Any() ResourceMatchers {
	return &matchAny{}
}

type matchAny struct{}

func (m *matchAny) RequireAllMatched() error      { return nil }
func (m *matchAny) Match(*yaml.ResourceMeta) bool { return true }

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

// Match returns true if any matches matches the given object
func (m resourceMatchers) Match(o *yaml.ResourceMeta) bool {
	for _, e := range m {
		if matchSelector(&e.ResourceSelector, o) {
			e.Matched = true
			return true
		}
	}
	return false
}

// matchSelector returns true if all non-empty fields of the selector match the ones in the provided object
func matchSelector(id *config.ResourceSelector, o *yaml.ResourceMeta) bool {
	return (id.APIVersion == "" || id.APIVersion == o.APIVersion) &&
		(id.Kind == "" || id.Kind == o.Kind) &&
		(id.Namespace == "" || id.Namespace == o.Namespace) &&
		(id.Name == "" || id.Name == o.Name)
}

// FromResourceSelectors creates matchers from the provided selectors
func FromResourceSelectors(selectors []config.ResourceSelector) ResourceMatchers {
	matchers := make([]*resourceMatcher, len(selectors))
	for i, selector := range selectors {
		matchers[i] = &resourceMatcher{selector, false}
	}
	return resourceMatchers(matchers)
}

// ChartHookMatcher matches chart hook resources when the delegated matcher doesn't match
type ChartHookMatcher struct {
	ResourceMatchers
	delegateOnly bool
	hooks        map[string]struct{}
}

// NewChartHookMatcher creates
func NewChartHookMatcher(delegate ResourceMatchers, delegateOnly bool) *ChartHookMatcher {
	return &ChartHookMatcher{
		ResourceMatchers: delegate,
		delegateOnly:     delegateOnly,
		hooks:            map[string]struct{}{},
	}
}

// FoundHooks returns all hooks that weren't matched by the delegate matcher
func (m *ChartHookMatcher) FoundHooks() []string {
	hooks := make([]string, 0, len(m.hooks))
	for hook := range m.hooks {
		hooks = append(hooks, hook)
	}
	sort.Strings(hooks)
	return hooks
}

// Match returns true if any matches matches the given object
func (m *ChartHookMatcher) Match(o *yaml.ResourceMeta) bool {
	if m.ResourceMatchers.Match(o) {
		return true
	}

	isHook := false
	if a := o.Annotations; a != nil {
		for _, hook := range strings.Split(a[annotationHelmHook], ",") {
			if hook = strings.TrimSpace(hook); hook != "" {
				m.hooks[hook] = struct{}{}
				isHook = true
			}
		}
	}
	return isHook && !m.delegateOnly
}
