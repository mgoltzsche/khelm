package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/repo"
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
			[]string{filepath.Join(exampleDir, "namespace")},
			3, "myconfigb",
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
			[]string{filepath.Join(exampleDir, "release-name"), "--name=myrelease"},
			1, "myrelease-config",
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
			[]string{filepath.Join(exampleDir, "release-name"),
				"--kube-version=1.17"},
			1, "k8sVersion: v1.17.0",
		},
		{
			"namespace",
			[]string{filepath.Join(exampleDir, "namespace"), "--namespace=mynamespace"},
			3, "namespace: mynamespace",
		},
		{
			"force-namespace",
			[]string{filepath.Join(exampleDir, "force-namespace"), "--force-namespace=forced-namespace"},
			5, "namespace: forced-namespace",
		},
		{
			"chart-hooks",
			[]string{filepath.Join(exampleDir, "chart-hooks")},
			10, "helm.sh/hook",
		},
		{
			"chart-hooks-excluded",
			[]string{filepath.Join(exampleDir, "chart-hooks"), "--no-hooks"},
			1, "myvalue",
		},
		{
			"git-dependency",
			[]string{filepath.Join(exampleDir, "git-dependency"), "--enable-git-getter", "--trust-any-repo"},
			24, "ca-sync",
		},
		{
			"local-chart-with-transitive-remote-and-git-dependencies",
			[]string{filepath.Join(exampleDir, "localrefref-with-git"), "--enable-git-getter", "--trust-any-repo"},
			33, "admission.certmanager.k8s.io",
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
	dir, err := os.MkdirTemp("", "khelm-tpl-test-")
	require.NoError(t, err)
	repoDir := filepath.Join(dir, "repository")
	defer os.RemoveAll(dir)
	os.Setenv("HELM_HOME", dir)
	defer os.Unsetenv("HELM_HOME")
	err = os.Mkdir(repoDir, 0755)
	require.NoError(t, err)
	err = repo.NewFile().WriteFile(filepath.Join(repoDir, "repositories.yaml"), 0644)
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
		{
			"reject git urls by default",
			[]string{"git-dependency", "--enable-git-getter=false"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			os.Args = append([]string{"testee", "template"}, c.args...)
			err := Execute(nil, &bytes.Buffer{})
			require.Error(t, err)
		})
	}
}
