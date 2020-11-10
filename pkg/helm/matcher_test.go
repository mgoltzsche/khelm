package helm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestMatchAny(t *testing.T) {
	input := []*yaml.ResourceMeta{}
	for _, suffix := range []string{"a", "b"} {
		input = append(input, testResource("some/version", "MyKind", "name"+suffix, "mynamespace"))
	}
	input = append(input, testResource("other/version", "OtherKind", "namec", "mynamespace"))
	input = append(input, testResource("other/version", "OtherKind", "namec", "othernamespace"))

	for _, c := range []struct {
		selectors      []ResourceSelector
		matchedCount   int
		containedNames []string
	}{
		{nil, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]ResourceSelector{{Name: "namec"}}, 2, []string{"namea", "nameb"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "NonExisting"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "MyKind"}}, 2, []string{"namec"}},
		{[]ResourceSelector{{Namespace: "mynamespace"}}, 3, []string{"namec"}},
		{[]ResourceSelector{{Namespace: "othernamespace"}}, 1, []string{"namea", "nameb"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]ResourceSelector{{APIVersion: "some/versionx", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "MyKindx", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespacex", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "nameax"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]ResourceSelector{{Name: "namea"}, {Name: "namec"}}, 3, []string{"nameb"}},
	} {
		testee := Matchers(c.selectors)
		matched := []string{}
		for _, o := range input {
			if testee.MatchAny(o) {
				matched = append(matched, o.Name)
			}
		}
		require.Equal(t, c.matchedCount, len(matched), "selector: %#v\n\tmatched: %+v", c.selectors, matched)
	}
}

func TestRequireAllMatched(t *testing.T) {
	testee := Matchers([]ResourceSelector{{Name: "myresource"}})
	matched := testee.MatchAny(testResource("someapi/v1", "SomeKind", "no-match", ""))
	require.False(t, matched, "matched")
	err := testee.RequireAllMatched()
	require.Error(t, err)
	_ = testee.MatchAny(testResource("someapi/v1", "SomeKind", "myresource", ""))
	err = testee.RequireAllMatched()
	require.NoError(t, err)
}

func testResource(apiVersion, kind, name, namespace string) *yaml.ResourceMeta {
	return &yaml.ResourceMeta{TypeMeta: yaml.TypeMeta{
		APIVersion: apiVersion,
		Kind:       kind,
	},
		ObjectMeta: yaml.ObjectMeta{NameMeta: yaml.NameMeta{
			Name:      name,
			Namespace: namespace,
		}}}
}
