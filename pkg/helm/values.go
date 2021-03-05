package helm

import (
	"bytes"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

const (
	generatorConfigValuesURLScheme = "generatorconfig"
	generatorConfigValuesURL       = generatorConfigValuesURLScheme + ":values"
)

func loadValues(cfg *config.ChartConfig, getters getter.Providers) (map[string]interface{}, error) {
	valueFiles := absPaths(cfg.ValueFiles, cfg.BaseDir)
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
		return nil, errors.Wrap(err, "load values: %w")
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
				return buf, errors.Wrap(err, "marshal helm values from generator config")
			}
			_, err = buf.Write(b)
			if err != nil {
				return buf, err
			}
		}
		return buf, nil
	}
	return buf, errors.Errorf("unsupported URL %q provided to generator config values getter", url)
}
