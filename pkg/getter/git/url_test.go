package git

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitURL(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  string
		expect gitURL
	}{
		{
			name:  "repo",
			input: "git+https://example.org/git/org/user",
			expect: gitURL{
				Repo: "https://example.org/git/org/user",
			},
		},
		{
			name:  "repo path",
			input: "git+https://example.org/git/org/user@some/path",
			expect: gitURL{
				Repo: "https://example.org/git/org/user",
				Path: "some/path",
			},
		},
		{
			name:  "repo ref",
			input: "git+https://example.org/git/org/user?ref=v1.2.3",
			expect: gitURL{
				Repo: "https://example.org/git/org/user",
				Ref:  "v1.2.3",
			},
		},
		{
			name:  "repo path ref",
			input: "git+https://example.org/git/org/user@some/path?ref=v1.2.3",
			expect: gitURL{
				Repo: "https://example.org/git/org/user",
				Ref:  "v1.2.3",
				Path: "some/path",
			},
		},
		{
			name:  "abs path",
			input: "git+https://example.org/git/org/user@/some/path?ref=v1.2.3",
			expect: gitURL{
				Repo: "https://example.org/git/org/user",
				Ref:  "v1.2.3",
				Path: "/some/path",
			},
		},
		{
			name:  "ssh",
			input: "git+ssh://git@example.org/git/org/user@some/path?ref=v1.2.3",
			expect: gitURL{
				Repo: "ssh://git@example.org/git/org/user",
				Ref:  "v1.2.3",
				Path: "some/path",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := parseURL(tc.input)
			require.NoError(t, err)
			require.Equal(t, &tc.expect, actual, "parseURL()")
			require.Equal(t, tc.input[4:], actual.String(), "url.String()")
		})
	}
}
func TestGitURLJoinPath(t *testing.T) {
	u := gitURL{
		Repo: "ssh://example.org/repo",
		Ref:  "v1.2.3",
		Path: "some/path",
	}
	n := u.JoinPath("other", "path")
	require.Equal(t, "some/path/other/path", n.Path)
	require.Equal(t, "some/path", u.Path)
}
