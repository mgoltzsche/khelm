package helm

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestYaml(t *testing.T) {
	obj := testee()
	y := obj.Yaml()
	um, err := unmarshal(y)
	require.NoError(t, err, "parsing Yaml() output")
	require.Equal(t, len(obj), len(um), "len")
	for i, o := range obj {
		require.Equal(t, o.Kind, um[i]["kind"], "kind (yaml: %q)", y)
		require.Equal(t, o.Raw["someattr"], um[i]["someattr"], "someattr")
	}
	require.Equal(t, y, obj.Yaml())
}

func TestParseObjects(t *testing.T) {
	expected := testee()
	actual, err := ParseObjects(bytes.NewReader([]byte(expected.Yaml())))
	require.NoError(t, err)
	require.Equal(t, len(expected), len(actual), "len")
	for i, o := range actual {
		require.Equal(t, expected[i].Raw["apiVersion"], o.Raw["apiVersion"], "apiVersion")
		expected[i].Raw = nil
		o.Raw = nil
		require.Equal(t, expected[i], o, "object id")
	}
}

func unmarshal(str string) (obj []map[string]interface{}, err error) {
	dec := yaml.NewDecoder(bytes.NewReader([]byte(str)))
	m := map[string]interface{}{}
	for ; err == nil; err = dec.Decode(m) {
		if len(m) > 0 {
			obj = append(obj, m)
			m = map[string]interface{}{}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func TestRemove(t *testing.T) {
	for _, c := range []struct {
		selectors      []K8sObjectID
		matchedCount   int
		containedNames []string
	}{
		{nil, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]K8sObjectID{{Name: "namec"}}, 2, []string{"namea", "nameb"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "NonExisting"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "MyKind"}}, 2, []string{"namec"}},
		{[]K8sObjectID{{Namespace: "mynamespace"}}, 3, []string{"namec"}},
		{[]K8sObjectID{{Namespace: "othernamespace"}}, 1, []string{"namea", "nameb"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]K8sObjectID{{APIVersion: "some/versionx", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "MyKindx", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespacex", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "nameax"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]K8sObjectID{{Name: "namea"}, {Name: "namec"}}, 3, []string{"nameb"}},
	} {
		obj := testee()
		l := len(obj)
		obj.Remove(Matchers(c.selectors))
		matched := l - len(obj)
		y := obj.Yaml()
		for _, name := range c.containedNames {
			require.Contains(t, y, name, "name did not match %#v", c.selectors)
		}
		require.Equal(t, c.matchedCount, matched, "%#v", c.selectors)
	}
}

func testee() (obj K8sObjects) {
	for _, suffix := range []string{"a", "b"} {
		obj = append(obj, &K8sObject{K8sObjectID: K8sObjectID{
			APIVersion: "some/version",
			Kind:       "MyKind",
			Name:       "name" + suffix,
			Namespace:  "mynamespace",
		}})
	}
	obj = append(obj, &K8sObject{K8sObjectID: K8sObjectID{
		APIVersion: "other/version",
		Kind:       "OtherKind",
		Name:       "namec",
		Namespace:  "mynamespace",
	}})
	obj = append(obj, &K8sObject{K8sObjectID: K8sObjectID{
		APIVersion: "other/version",
		Kind:       "OtherKind",
		Name:       "namec",
		Namespace:  "othernamespace",
	}})
	for _, o := range obj {
		o.Raw = map[string]interface{}{
			"apiVersion": o.APIVersion,
			"kind":       o.Kind,
			"metadata": map[string]interface{}{
				"name":      o.Name,
				"namespace": o.Namespace,
			},
			"someattr": 7,
		}
	}
	return
}
