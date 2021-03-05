package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mgoltzsche/khelm/v2/internal/output"
	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestKptFnCommand(t *testing.T) {
	dir, err := ioutil.TempDir("", "khelm-fn-test-")
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
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
			}},
			3, "myconfiga",
		},
		{
			"latest cluster scoped remote chart",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
				},
			}},
			-1, "acme.cert-manager.io",
		},
		{
			"remote chart with version",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
					Version:    "0.9.x",
				},
			}},
			34, "chart: cainjector-v0.9.1",
		},
		{
			"release name",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "release-name"),
				},
				RendererConfig: config.RendererConfig{
					Name: "myrelease",
				},
			}},
			1, "myrelease-config",
		},
		{
			"valueFiles",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					ValueFiles: []string{filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
				}}},
			1, " valueoverwrite: overwritten by file",
		},
		{
			"values",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					Values: map[string]interface{}{
						"example": map[string]string{"overrideValue": "explicitly"},
					},
				}}},
			1, " valueoverwrite: explicitly",
		},
		{
			"values override",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					ValueFiles: []string{filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
					Values: map[string]interface{}{
						"example": map[string]string{"overrideValue": "explicitly"},
					},
				}}},
			1, " valueoverwrite: explicitly",
		},
		{
			"apiversions",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "apiversions-condition", "chart"),
				},
				RendererConfig: config.RendererConfig{
					APIVersions: []string{"myfancyapi/v1", ""},
				}}},
			1, "fancycr",
		},
		{
			"kubeversion",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "release-name"),
				},
				RendererConfig: config.RendererConfig{
					KubeVersion: "1.12",
				}}},
			1, "k8sVersion: v1.12.0",
		},
		{
			"namespace",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					Namespace: "mynamespace",
				},
			}},
			3, " namespace: mynamespace\n",
		},
		{
			"force namespace",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					ForceNamespace: "forced-namespace",
				},
			}},
			3, " namespace: forced-namespace\n",
		},
		{
			"exclude",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					Exclude: []config.ResourceSelector{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
							Name:       "myconfiga",
						},
					},
				},
			}},
			2, "myconfigb",
		},
		{
			"include",
			kptFnConfig{ChartConfig: &config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					Include: []config.ResourceSelector{
						{
							APIVersion: "v1",
							Kind:       "ConfigMap",
						},
					},
					Exclude: []config.ResourceSelector{
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
				ChartConfig: &config.ChartConfig{
					LoaderConfig: config.LoaderConfig{
						Chart: filepath.Join(exampleDir, "namespace"),
					},
				},
				OutputPath: "my/output/manifest.yaml",
			},
			3, "  config.kubernetes.io/path: my/output/manifest.yaml\n",
		},
		{
			"output kustomization",
			kptFnConfig{
				ChartConfig: &config.ChartConfig{
					LoaderConfig: config.LoaderConfig{
						Chart: filepath.Join(exampleDir, "namespace"),
					},
				},
				OutputPath: "my/output/path/",
			},
			4, "resources:\n  - configmap_myconfiga.yaml\n  - configmap_myconfigb.yaml\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.input.Debug = true
			if c.input.Name == "" {
				c.input.Name = "release-name"
			}
			outPath := c.input.OutputPath
			if outPath == "" {
				if output.IsDirectory(outPath) {
					outPath = path.Join(outPath, "previously-generated.yaml")
				} else {
					outPath = "generated-manifest.yaml"
				}
			}
			inputAnnotations[annotationPath] = outPath
			b, err := yaml.Marshal(map[string]interface{}{
				"apiVersion":     "config.kubernetes.io/v1alpha1",
				"kind":           "ResourceList",
				"items":          inputItems,
				"functionConfig": map[string]interface{}{"data": c.input},
			})
			require.NoError(t, err)
			var out bytes.Buffer
			os.Args = []string{"khelmfn"}
			err = Execute(bytes.NewReader(b), &out)
			require.NoError(t, err)
			result := validateYAML(t, out.Bytes(), 1)
			items, _ := result["items"].([]interface{})
			out.Reset()
			enc := yaml.NewEncoder(&out)
			for _, item := range items {
				err = enc.Encode(item)
				require.NoError(t, err)
			}
			if c.mustContainObj >= 0 {
				// assert n+1 resources because one provided input resource should be preserved,
				// the 2nd input resource should be excluded since it was generated by this function during a previous run.
				if !assert.Equal(t, c.mustContainObj+1, len(items), "amount of resources within output") {
					t.Log("\n" + out.String())
				}
			}
			require.Contains(t, out.String(), c.mustContain, "output of %#v", c.input)
		})
	}
}
