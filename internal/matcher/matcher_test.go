package matcher

import (
	"testing"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestMatchAll(t *testing.T) {
	testee := Any()
	resID := testResource("someapi/v1", "SomeKind", "no-match", "").GetIdentifier()
	matched := testee.Match(&resID)
	require.True(t, matched, "matched")
	err := testee.RequireAllMatched()
	require.NoError(t, err, "RequireAllMatched")
}

func TestMatchAny(t *testing.T) {
	input := []*yaml.ResourceMeta{}
	for _, suffix := range []string{"a", "b"} {
		input = append(input, testResource("some/version", "MyKind", "name"+suffix, "mynamespace"))
	}
	input = append(input, testResource("other/version", "OtherKind", "namec", "mynamespace"))
	input = append(input, testResource("other/version", "OtherKind", "namec", "othernamespace"))

	for _, c := range []struct {
		selectors      []config.ResourceSelector
		matchedCount   int
		containedNames []string
	}{
		{nil, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]config.ResourceSelector{{Name: "namec"}}, 2, []string{"namea", "nameb"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "NonExisting"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "MyKind"}}, 2, []string{"namec"}},
		{[]config.ResourceSelector{{Namespace: "mynamespace"}}, 3, []string{"namec"}},
		{[]config.ResourceSelector{{Namespace: "othernamespace"}}, 1, []string{"namea", "nameb"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 1, []string{"nameb", "namec"}},
		{[]config.ResourceSelector{{APIVersion: "some/versionx", Kind: "MyKind", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "MyKindx", Namespace: "mynamespace", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespacex", Name: "namea"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{APIVersion: "some/version", Kind: "MyKind", Namespace: "mynamespace", Name: "nameax"}}, 0, []string{"namea", "nameb", "namec"}},
		{[]config.ResourceSelector{{Name: "namea"}, {Name: "namec"}}, 3, []string{"nameb"}},
	} {
		testee := FromResourceSelectors(c.selectors)
		matched := []string{}
		for _, o := range input {
			resID := o.GetIdentifier()
			if testee.Match(&resID) {
				matched = append(matched, o.Name)
			}
		}
		require.Equal(t, c.matchedCount, len(matched), "selector: %#v\n\tmatched: %+v", c.selectors, matched)
	}
}

func TestRequireAllMatched(t *testing.T) {
	testee := FromResourceSelectors([]config.ResourceSelector{{Name: "myresource1"}, {Name: "myresource2"}})
	input := testResource("someapi/v1", "SomeKind", "no-match", "").GetIdentifier()
	matched := testee.Match(&input)
	require.False(t, matched, "matched")
	err := testee.RequireAllMatched()
	require.Error(t, err)
	input = testResource("someapi/v1", "SomeKind", "myresource1", "").GetIdentifier()
	matched = testee.Match(&input)
	require.True(t, matched, "matched")
	err = testee.RequireAllMatched()
	require.Error(t, err)
	input = testResource("someapi/v1", "SomeKind", "myresource2", "").GetIdentifier()
	matched = testee.Match(&input)
	require.True(t, matched, "matched")
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
