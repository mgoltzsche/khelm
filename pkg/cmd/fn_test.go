package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestKptFnCommand(t *testing.T) {
	dir, err := ioutil.TempDir("", "helmr-fn-test-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	os.Setenv("HELM_HOME", dir)
	defer os.Unsetenv("HELM_HOME")
	exampleDir := filepath.Join("..", "..", "example")

	inputAnnotations := map[string]interface{}{}
	inputItems := []map[string]interface{}{
		{
			// should be preserved
			"somekey": "somevalue",
		},
		{
			// should be filtered
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"annotations": inputAnnotations},
		},
	}

	for _, c := range []struct {
		name           string
		input          kptFnConfig
		mustContainObj int
		mustContain    string
	}{
		{
			"chart path only",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "no-namespace"),
				},
			}},
			2, "myconfiga",
		},
		{
			"latest cluster scoped remote chart",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
				},
			}},
			-1, "acme.cert-manager.io",
		},
		{
			"remote chart with version",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
					Version:    "0.9.x",
				},
			}},
			34, "chart: cainjector-v0.9.1",
		},
		{
			"release name",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "no-namespace"),
				},
				RendererConfig: helm.RendererConfig{
					Name: "myrelease",
				},
			}},
			2, "myrelease-myconfigb",
		},
		{
			"valueFiles",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: helm.RendererConfig{
					ValueFiles: []string{filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
				}}},
			1, "overwritten by file",
		},
		{
			"values",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: helm.RendererConfig{
					Values: map[string]interface{}{
						"example": map[string]string{"overrideValue": "explicitly"},
					},
				}}},
			1, "explicitly",
		},
		{
			"apiversions",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "apiversions-condition", "chart"),
				},
				RendererConfig: helm.RendererConfig{
					APIVersions: []string{"myfancyapi/v1", ""},
				}}},
			1, "fancycr",
		},
		{
			"kubeversion",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "apiversions-condition", "chart"),
				},
				RendererConfig: helm.RendererConfig{
					APIVersions: []string{"myfancyapi/v1", ""},
					KubeVersion: "1.12",
				}}},
			1, "k8sVersion: v1.12.0",
		},
		{
			"namespace",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "no-namespace"),
				},
				RendererConfig: helm.RendererConfig{
					Namespace: "mynamespace",
				},
			}},
			2, "namespace: mynamespace",
		},
		{
			"exclude",
			kptFnConfig{ChartConfig: &helm.ChartConfig{
				LoaderConfig: helm.LoaderConfig{
					Chart: filepath.Join(exampleDir, "no-namespace"),
				},
				RendererConfig: helm.RendererConfig{
					Exclude: []helm.ResourceSelector{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "myconfiga",
						},
					},
				},
			}},
			1, "myconfigb",
		},
		{
			"output path",
			kptFnConfig{
				ChartConfig: &helm.ChartConfig{
					LoaderConfig: helm.LoaderConfig{
						Chart: filepath.Join(exampleDir, "no-namespace"),
					},
				},
				OutputPath: "my/output/path",
			},
			2, "  config.kubernetes.io/path: my/output/path/configmap_release-name-myconfigb.yaml\n",
		},
		{
			"output kustomization",
			kptFnConfig{
				ChartConfig: &helm.ChartConfig{
					LoaderConfig: helm.LoaderConfig{
						Chart: filepath.Join(exampleDir, "no-namespace"),
					},
				},
				OutputPath:          "my/output/path",
				OutputKustomization: true,
			},
			3, "resources:\n  - configmap_myconfiga.yaml\n  - configmap_release-name-myconfigb.yaml\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.input.Name == "" {
				c.input.Name = "release-name"
			}
			outPath := c.input.OutputPath
			if outPath == "" {
				outPath = "chart-output"
			}
			inputAnnotations[annotationPath] = path.Join(outPath, "previously-generated.yaml")
			b, err := yaml.Marshal(map[string]interface{}{
				"apiVersion":     "config.kubernetes.io/v1alpha1",
				"kind":           "ResourceList",
				"items":          inputItems,
				"functionConfig": map[string]interface{}{"data": c.input},
			})
			require.NoError(t, err)
			var out bytes.Buffer
			os.Args = []string{"helmrfn"}
			err = Execute(bytes.NewReader(b), &out)
			require.NoError(t, err)
			result := validateYAML(t, out.Bytes(), 1)
			items, _ := result["items"].([]interface{})
			if c.mustContainObj >= 0 {
				require.Equal(t, c.mustContainObj, len(items)-1, "amount of resources within output")
			}
			out.Reset()
			enc := yaml.NewEncoder(&out)
			for _, item := range items {
				err = enc.Encode(item)
				require.NoError(t, err)
			}
			require.Contains(t, out.String(), c.mustContain, "output of %#v", c.input)
		})
	}
}
