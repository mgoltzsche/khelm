package helm

import (
	"io"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const (
	generatorAPIVersion = "helm.kustomize.mgoltzsche.github.com/v1"
	generatorKind       = "ChartRenderer"
)

// GeneratorConfig define the kustomize plugin's input file content
type GeneratorConfig struct {
	APIVersion  string      `yaml:"apiVersion"`
	Kind        string      `yaml:"kind"`
	Metadata    K8sMetadata `yaml:"metadata"`
	ChartConfig `yaml:",inline"`
}

// K8sMetadata define the name to be kubernetes object schema conform
type K8sMetadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

// ChartConfig define chart lookup and render config
type ChartConfig struct {
	LoadChartConfig `yaml:",inline"`
	RenderConfig    `yaml:",inline"`
}

// LoadChartConfig define the configuration to load a chart
type LoadChartConfig struct {
	Repository string `yaml:"repository,omitempty"`
	Chart      string `yaml:"chart"`
	Version    string `yaml:"version,omitempty"`
	Verify     bool   `yaml:"verify,omitempty"`
	Keyring    string `yaml:"keyring,omitempty"`
}

// RenderConfig defines the configuration to render a chart
type RenderConfig struct {
	Name        string                 `yaml:"name,omitempty"`
	Namespace   string                 `yaml:"namespace,omitempty"`
	ValueFiles  []string               `yaml:"valueFiles,omitempty"`
	Values      map[string]interface{} `yaml:"values,omitempty"`
	APIVersions []string               `yaml:"apiVersions,omitempty"`
	Exclude     []K8sObjectID          `yaml:"exclude,omitempty"`
	BaseDir     string                 `yaml:"-"`
	RootDir     string                 `yaml:"-"`
}

// ReadGeneratorConfig read the generator configuration
func ReadGeneratorConfig(reader io.Reader) (cfg *GeneratorConfig, err error) {
	cfg = &GeneratorConfig{}
	dec := yaml.NewDecoder(reader)
	dec.KnownFields(true)
	err = dec.Decode(cfg)
	if err == nil {
		e := dec.Decode(cfg)
		if e == nil {
			err = errors.New("contains more than one object")
		} else if e != io.EOF {
			err = e
		}
	}
	if err == nil {
		if cfg.Namespace == "" {
			cfg.Namespace = cfg.Metadata.Namespace
		} else if cfg.Metadata.Namespace != "" && err == nil {
			err = errors.New("both metadata.namespace and namespace defined")
		}
		if cfg.Version == "" && cfg.Repository != "" {
			err = errors.New("no chart version but repository specified")
		}
		if cfg.Chart == "" {
			err = errors.New("chart not specified")
		}
		if cfg.Kind != generatorKind {
			err = errors.Errorf("expected kind %s but was %s", generatorKind, cfg.Kind)
		}
		if cfg.APIVersion != generatorAPIVersion {
			err = errors.Errorf("expected apiVersion %s but was %s", generatorAPIVersion, cfg.APIVersion)
		}
	}
	return cfg, errors.Wrap(err, "read chart renderer config")
}
