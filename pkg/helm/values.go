package helm

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

const (
	generatorConfigValuesURLScheme = "generatorconfig"
	generatorConfigValuesURL       = generatorConfigValuesURLScheme + ":values"
)

func loadValues(cfg *RenderConfig, getters getter.Providers) (map[string]interface{}, error) {
	valueFiles, err := securePaths(cfg.ValueFiles, cfg.BaseDir, cfg.RootDir)
	if err != nil {
		return nil, err
	}
	valueGetters := append(getters, getter.Provider{
		Schemes: []string{generatorConfigValuesURLScheme},
		New: func(_ ...getter.Option) (getter.Getter, error) {
			return configValuesGetter(cfg.Values), nil
		},
	})
	valueOpts := &values.Options{
		ValueFiles: append(valueFiles, generatorConfigValuesURL),
	}
	vals, err := valueOpts.MergeValues(valueGetters)
	if err != nil {
		return nil, fmt.Errorf("load values: %w", err)
	}
	return vals, nil
}

type configValuesGetter map[string]interface{}

func (g configValuesGetter) Get(url string, options ...getter.Option) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	if url == generatorConfigValuesURL {
		if g != nil {
			b, err := yaml.Marshal(map[string]interface{}(g))
			if err != nil {
				return buf, fmt.Errorf("marshal helm values from generator config: %w", err)
			}
			_, err = buf.Write(b)
			if err != nil {
				return buf, err
			}
		}
		return buf, nil
	}
	return buf, fmt.Errorf("unsupported URL %q provided to generator config values getter", url)
}
