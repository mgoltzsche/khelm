package helm

import (
	"io"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// K8sObjectID specifies an object selector
type K8sObjectID struct {
	APIVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
	Name       string `yaml:"name,omitempty"`
}

// Match returns true if all non-empty fields match the ones in the provided object
func (id *K8sObjectID) Match(obj *K8sObject) bool {
	return (id.APIVersion == "" || id.APIVersion == obj.APIVersion) &&
		(id.Kind == "" || id.Kind == obj.Kind) &&
		(id.Namespace == "" || id.Namespace == obj.Namespace) &&
		(id.Name == "" || id.Name == obj.Name)
}

// K8sObject represents a matchable k8s API object
type K8sObject struct {
	K8sObjectID
	Raw map[string]interface{}
}

// MatchAny returns true if any of the provided selectors match
func (o *K8sObject) MatchAny(matchers []*K8sObjectMatcher) bool {
	for _, m := range matchers {
		if m.Match(o) {
			return true
		}
	}
	return false
}

// K8sObjectMatcher matches
type K8sObjectMatcher struct {
	K8sObjectID
	Matched bool
}

// Matchers creates a list of matchers from the provided selectors
func Matchers(selectors []K8sObjectID) []*K8sObjectMatcher {
	matchers := make([]*K8sObjectMatcher, len(selectors))
	for i, selector := range selectors {
		matchers[i] = &K8sObjectMatcher{selector, false}
	}
	return matchers
}

// Match returns true if the provided object matches the selector
func (m *K8sObjectMatcher) Match(o *K8sObject) bool {
	if m.K8sObjectID.Match(o) {
		m.Matched = true
		return true
	}
	return false
}

// K8sObjects represents a list of k8s API objects
type K8sObjects []*K8sObject

// Remove removes all API objects from the contained list that match any of the provided selectors
func (obj *K8sObjects) Remove(matcher []*K8sObjectMatcher) {
	filtered := []*K8sObject{}
	for _, o := range *obj {
		if !o.MatchAny(matcher) {
			filtered = append(filtered, o)
		}
	}
	*obj = K8sObjects(filtered)
}

// Yaml renders the object list to YAML
func (obj K8sObjects) Yaml() string {
	y := ""
	for _, o := range obj {
		b, err := yaml.Marshal(o.Raw)
		if err != nil {
			panic(err)
		}
		y += "---\n" + string(b)
	}
	return y
}

// ParseObjects parses k8s API objects
func ParseObjects(f io.Reader) (obj K8sObjects, err error) {
	var o *K8sObject
	dec := yaml.NewDecoder(f)
	m := map[string]interface{}{}
	for ; err == nil; err = dec.Decode(m) {
		if len(m) > 0 {
			if o, err = objFromMap(m); err != nil {
				return
			}
			obj = append(obj, o)
			m = map[string]interface{}{}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func objFromMap(m map[string]interface{}) (r *K8sObject, err error) {
	var o K8sObject
	apiVersion, hasAPIVersion := m["apiVersion"].(string)
	kind, hasKind := m["kind"].(string)
	meta, err := asMap(m["metadata"])
	if err != nil {
		return
	}
	name, hasName := meta["name"].(string)
	o.Namespace, _ = meta["namespace"].(string)
	if !hasName || name == "" {
		err = errors.Errorf("object has no name of type string: %#v", m)
	}
	if !hasKind || kind == "" {
		err = errors.Errorf("object has no kind of type string: %#v", m)
	}
	if !hasAPIVersion || apiVersion == "" {
		err = errors.Errorf("object has no apiVersion of type string: %#v", m)
	}
	o.APIVersion = apiVersion
	o.Kind = kind
	o.Name = name
	o.Raw = m
	return &o, err
}

func asMap(o interface{}) (m map[string]interface{}, err error) {
	if o != nil {
		if mc, ok := o.(map[string]interface{}); ok {
			return mc, nil
		} else if mc, ok := o.(map[interface{}]interface{}); ok {
			m = map[string]interface{}{}
			for k, v := range mc {
				m[k.(string)] = v
			}
			return
		}
	}
	return nil, errors.New("invalid metadata")
}
