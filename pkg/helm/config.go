package helm

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const (
	// GeneratorAPIVersion specifies the apiVersion field value supported by the generator
	GeneratorAPIVersion = "helm.kustomize.mgoltzsche.github.com/v1"
	// GeneratorKind specifies the API kind field value supported by the generator
	GeneratorKind = "ChartRenderer"
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
	LoaderConfig   `yaml:",inline"`
	RendererConfig `yaml:",inline"`
	BaseDir        string `yaml:"-"`
	RootDir        string `yaml:"-"`
}

// LoaderConfig define the configuration to load a chart
type LoaderConfig struct {
	Repository string `yaml:"repository,omitempty"`
	Chart      string `yaml:"chart"`
	Version    string `yaml:"version,omitempty"`
	Verify     bool   `yaml:"verify,omitempty"`
	Keyring    string `yaml:"keyring,omitempty"`
}

// RendererConfig defines the configuration to render a chart
type RendererConfig struct {
	Name        string                 `yaml:"name,omitempty"` // deprecated releaseName alias
	ReleaseName string                 `yaml:"releaseName,omitempty"`
	Namespace   string                 `yaml:"namespace,omitempty"`
	ValueFiles  []string               `yaml:"valueFiles,omitempty"`
	Values      map[string]interface{} `yaml:"values,omitempty"`
	APIVersions []string               `yaml:"apiVersions,omitempty"`
	Exclude     []K8sObjectID          `yaml:"exclude,omitempty"`
}

// ReadGeneratorConfig read the generator configuration
func ReadGeneratorConfig(reader io.Reader) (cfg *GeneratorConfig, err error) {
	cfg = &GeneratorConfig{}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	err = dec.Decode(cfg)
	if err != nil {
		// Accept unknown fields but warn about them
		dec = yaml.NewDecoder(bytes.NewReader(data))
		e := dec.Decode(cfg)
		if e == nil {
			log.Printf("WARNING: chart %s contains unsupported fields: %s", cfg.Metadata.Name, err)
			err = nil
		}
	}
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
		if cfg.ReleaseName == "" && cfg.Name != "" {
			log.Printf("WARNING: chart config %q field \"name\" is deprecated in favour of \"releaseName\"", cfg.Metadata.Name)
			cfg.ReleaseName = cfg.Name
		}
		if cfg.ReleaseName == "" {
			cfg.ReleaseName = cfg.Metadata.Name
		}
		if cfg.ReleaseName == "" {
			err = errors.New("releaseName not specified")
		}
		if cfg.Version == "" && cfg.Repository != "" {
			err = errors.New("no chart version but repository specified")
		}
		if cfg.Chart == "" {
			err = errors.New("chart not specified")
		}
		if cfg.Kind != GeneratorKind {
			err = errors.Errorf("expected kind %s but was %s", GeneratorKind, cfg.Kind)
		}
		if cfg.APIVersion != GeneratorAPIVersion {
			err = errors.Errorf("expected apiVersion %s but was %s", GeneratorAPIVersion, cfg.APIVersion)
		}
	}
	return cfg, errors.Wrap(err, "read chart renderer config")
}