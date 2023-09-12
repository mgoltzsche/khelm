package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/mgoltzsche/khelm/v2/internal/output"
	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func TestKptFnCommand(t *testing.T) {
	dir, err := os.MkdirTemp("", "khelm-fn-test-")
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

	origK8sVersion := chartutil.DefaultCapabilities.KubeVersion.Version
	defer func() {
		chartutil.DefaultCapabilities.KubeVersion.Version = origK8sVersion
	}()

	for _, c := range []struct {
		name           string
		input          config.KRMFuncConfig
		mustContainObj int
		mustContain    []string
	}{
		{
			"chart path only",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
			}},
			3, []string{"myconfiga"},
		},
		{
			"latest cluster scoped remote chart",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
				},
				RendererConfig: config.RendererConfig{
					KubeVersion: "v1.27.0",
				},
			}},
			-1, []string{"acme.cert-manager.io"},
		},
		{
			"remote chart with version",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Repository: "https://charts.jetstack.io",
					Chart:      "cert-manager",
					Version:    "0.9.x",
				},
			}},
			34, []string{"chart: cainjector-v0.9.1"},
		},
		{
			"release name",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "release-name"),
				},
				RendererConfig: config.RendererConfig{
					Name: "myrelease",
				},
			}},
			1, []string{"myrelease-config"},
		},
		{
			"valueFiles",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					ValueFiles: []string{filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
				}}},
			1, []string{" valueoverwrite: overwritten by file"},
		},
		{
			"values",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					Values: map[string]interface{}{
						"example": map[string]string{"overrideValue": "explicitly"},
					},
				}}},
			1, []string{" valueoverwrite: explicitly"},
		},
		{
			"values override",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "values-inheritance", "chart"),
				},
				RendererConfig: config.RendererConfig{
					ValueFiles: []string{filepath.Join(exampleDir, "values-inheritance", "values.yaml")},
					Values: map[string]interface{}{
						"example": map[string]string{"overrideValue": "explicitly"},
					},
				}}},
			1, []string{" valueoverwrite: explicitly"},
		},
		{
			"apiversions",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "apiversions-condition", "chart"),
				},
				RendererConfig: config.RendererConfig{
					APIVersions: []string{"myfancyapi/v1", ""},
				}}},
			1, []string{"fancycr"},
		},
		{
			"kubeversion",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "release-name"),
				},
				RendererConfig: config.RendererConfig{
					KubeVersion: "1.12",
				}}},
			1, []string{"k8sVersion: v1.12.0"},
		},
		{
			"expand-list",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "expand-list"),
				},
			}},
			3, []string{"\n  name: myserviceaccount2\n"},
		},
		{
			"namespace",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					Namespace: "mynamespace",
				},
			}},
			3, []string{" namespace: mynamespace\n"},
		},
		{
			"force namespace",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
				LoaderConfig: config.LoaderConfig{
					Chart: filepath.Join(exampleDir, "namespace"),
				},
				RendererConfig: config.RendererConfig{
					ForceNamespace: "forced-namespace",
				},
			}},
			3, []string{" namespace: forced-namespace\n"},
		},
		{
			"exclude",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
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
			2, []string{"myconfigb"},
		},
		{
			"include",
			config.KRMFuncConfig{ChartConfig: config.ChartConfig{
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
			1, []string{"myconfigb"},
		},
		{
			"annotate output path",
			config.KRMFuncConfig{
				ChartConfig: config.ChartConfig{
					LoaderConfig: config.LoaderConfig{
						Chart: filepath.Join(exampleDir, "namespace"),
					},
				},
				OutputPath: "my/output/manifest.yaml",
			},
			3, []string{" annotations:\n    config.kubernetes.io/index: 0\n    config.kubernetes.io/path: my/output/manifest.yaml\n"},
		},
		{
			"annotate output path when annotations empty",
			config.KRMFuncConfig{
				ChartConfig: config.ChartConfig{
					LoaderConfig: config.LoaderConfig{
						Chart: filepath.Join(exampleDir, "empty-annotations"),
					},
				},
				OutputPath: "my/output/path/",
			},
			3, []string{
				"\n    config.kubernetes.io/path: my/output/path/kustomization.yaml\n",
				"\n    config.kubernetes.io/path: my/output/path/serviceaccount_sa1.yaml\n",
				"\n    config.kubernetes.io/path: my/output/path/serviceaccount_sa2.yaml\n",
				" myannotation: should-be-preserved\n",
			},
		},
		{
			"output kustomization",
			config.KRMFuncConfig{
				ChartConfig: config.ChartConfig{
					LoaderConfig: config.LoaderConfig{
						Chart: filepath.Join(exampleDir, "namespace"),
					},
				},
				OutputPath: "my/output/path/",
			},
			4, []string{"resources:\n- configmap_myconfiga.yaml\n- configmap_myconfigb.yaml\n"},
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
				"apiVersion": "config.kubernetes.io/v1alpha1",
				"kind":       "ResourceList",
				"items":      inputItems,
				"functionConfig": config.KRMFuncConfigFile{
					APIVersion:    "khelm.mgoltzsche.github.com/v2",
					Kind:          "ChartRenderer",
					KRMFuncConfig: c.input,
				},
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
			for _, mustContain := range c.mustContain {
				require.Contains(t, out.String(), mustContain, "output of %#v", c.input)
			}
		})
	}
}
