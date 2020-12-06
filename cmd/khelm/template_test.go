package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/helm/pkg/repo"
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
			2, "myconfigb",
		},
		{
			"latest cluster scoped remote chart",
			[]string{"cert-manager", "--repo=https://charts.jetstack.io",
				"--trust-any-repo"},
			-1, "acme.cert-manager.io",
		},
		{
			"remote chart with version",
			[]string{"cert-manager", "--version=0.9.x",
				"--repository=https://charts.jetstack.io",
				"--repo=https://charts.jetstack.io",
				"--trust-any-repo"},
			34, "chart: cainjector-v0.9.1",
		},
		{
			"release name",
			[]string{filepath.Join(exampleDir, "no-namespace"), "--name=myrelease"},
			2, "myrelease-myconfigb",
		},
		{
			"values",
			[]string{filepath.Join(exampleDir, "values-inheritance", "chart"),
				"--values=" + filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
			1, " valueoverwrite: overwritten by file",
		},
		{
			"set",
			[]string{filepath.Join(exampleDir, "values-inheritance", "chart"),
				"--set=example.other1=a,example.overrideValue=explicitly,example.other2=b", "--set=example.other1=x"},
			1, " valueoverwrite: explicitly",
		},
		{
			"set override",
			[]string{filepath.Join(exampleDir, "values-inheritance", "chart"),
				"--values=" + filepath.Join(exampleDir, "values-inheritance", "values.yaml"),
				"--set=example.other1=a,example.overrideValue=explicitly,example.other2=b", "--set=example.other1=x"},
			1, " valueoverwrite: explicitly",
		},
		{
			"apiversions",
			[]string{filepath.Join(exampleDir, "apiversions-condition", "chart"),
				"--api-versions=myfancyapi/v1", "--api-versions=someapi/v1alpha1"},
			1, "fancycr",
		},
		{
			"kubeversion",
			[]string{filepath.Join(exampleDir, "no-namespace"),
				"--kube-version=1.17"},
			2, "k8sVersion: v1.17.0",
		},
		{
			"namespace",
			[]string{filepath.Join(exampleDir, "no-namespace"), "--namespace=mynamespace"},
			2, "namespace: mynamespace",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			os.Args = append([]string{"testee", "template"}, c.args...)
			err := Execute(nil, &out)
			require.NoError(t, err)
			validateYAML(t, out.Bytes(), c.mustContainObj)
			require.Contains(t, out.String(), c.mustContain, "output of %+v", c.args)
		})
	}
}

func TestTemplateCommandError(t *testing.T) {
	dir, err := ioutil.TempDir("", "khelm-tpl-test-")
	require.NoError(t, err)
	repoDir := filepath.Join(dir, "repository")
	defer os.RemoveAll(dir)
	os.Setenv("HELM_HOME", dir)
	defer os.Unsetenv("HELM_HOME")
	err = os.Mkdir(repoDir, 0755)
	require.NoError(t, err)
	err = repo.NewRepoFile().WriteFile(filepath.Join(repoDir, "repositories.yaml"), 0644)
	require.NoError(t, err)
	for _, c := range []struct {
		name string
		args []string
	}{
		{
			"reject untrusted repo",
			[]string{"cert-manager", "--repo=https://charts.jetstack.io"},
		},
		{
			"reject cluster scoped resources",
			[]string{"cert-manager", "--repo=https://charts.jetstack.io", "--namespaced-only"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			os.Args = append([]string{"testee", "template"}, c.args...)
			err := Execute(nil, &bytes.Buffer{})
			require.Error(t, err)
		})
	}
}
