package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateCommand(t *testing.T) {
	exampleDir := filepath.Join("..", "..", "example")
	for _, c := range []struct {
		name           string
		args           []string
		mustContainObj int
		mustContain    string
	}{
		{
			"chart path only",
			[]string{filepath.Join(exampleDir, "no-namespace")},
			2, "myconfigb"},
		{
			"latest cluster scoped remote chart",
			[]string{"cert-manager", "--repo=https://charts.jetstack.io",
				"--accept-any-repo", "--accept-cluster-scoped"},
			-1, "acme.cert-manager.io",
		},
		{
			"remote chart with version",
			[]string{"cert-manager", "--version=0.9.x",
				"--repo=https://charts.jetstack.io",
				"--accept-any-repo", "--accept-cluster-scoped"},
			34, "chart: cainjector-v0.9.1",
		},
		{
			"apiversions",
			[]string{filepath.Join(exampleDir, "apiversions-condition", "chart"),
				"--api-versions=myfancyapi/v1", "--api-versions=someapi/v1alpha1"},
			1, "fancycr",
		},
		{
			"values",
			[]string{filepath.Join(exampleDir, "values-inheritance", "chart"),
				"--values=" + filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
			1, "overwritten by file",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runTemplateCmd(t, c.args)
			validateYAML(t, out, c.mustContainObj)
			require.Contains(t, string(out), c.mustContain, "output of %+v", c.args)
		})
	}
}

func runTemplateCmd(t *testing.T, args []string) []byte {
	os.Args = append([]string{"testee", "template"}, args...)
	var buf bytes.Buffer
	err := Execute(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}
