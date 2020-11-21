package helm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/util/homedir"
	"k8s.io/helm/pkg/chartutil"
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
}

// NewChartConfig creates a new empty chart config with default values
func NewChartConfig() (cfg ChartConfig) {
	cfg.applyDefaults()
	return
}

func (cfg *ChartConfig) applyDefaults() {
	if cfg.Name == "" {
		cfg.Name = "release-name"
	}
	if cfg.KubeVersion == "" {
		cfg.KubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)
	}
	if cfg.Values == nil {
		cfg.Values = map[string]interface{}{}
	}
	if cfg.Keyring == "" {
		cfg.Keyring = filepath.Join(homedir.HomeDir(), ".gnupg", "pubring.gpg")
	}
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
	Name          string                 `yaml:"name,omitempty"`
	Namespace     string                 `yaml:"namespace,omitempty"`
	ValueFiles    []string               `yaml:"valueFiles,omitempty"`
	Values        map[string]interface{} `yaml:"values,omitempty"`
	KubeVersion   string                 `yaml:"kubeVersion,omitempty"`
	APIVersions   []string               `yaml:"apiVersions,omitempty"`
	Exclude       []ResourceSelector     `yaml:"exclude,omitempty"`
	ClusterScoped bool                   `yaml:"clusterScoped,omitempty"`
}

// Validate validates the chart renderer config
func (cfg *ChartConfig) Validate() (errs []string) {
	if cfg.Chart == "" {
		errs = append(errs, "chart not specified")
	}
	if cfg.Version == "" && cfg.Repository != "" {
		errs = append(errs, "no chart version but repository specified")
	}
	if cfg.Name == "" {
		errs = append(errs, "releaseName not specified")
	}
	return
}

// ReadGeneratorConfig read the generator configuration
func ReadGeneratorConfig(reader io.Reader) (cfg *GeneratorConfig, err error) {
	cfg = &GeneratorConfig{}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "read chart renderer config")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	err = dec.Decode(cfg)
	if err != nil {
		// Accept unknown fields but warn about them
		dec = yaml.NewDecoder(bytes.NewReader(data))
		e := dec.Decode(cfg)
		if e == nil {
			log.Printf("WARNING: chart renderer config %q contains unsupported fields: %s", cfg.Metadata.Name, err)
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
		}
		if cfg.Name == "" {
			cfg.Name = cfg.Metadata.Name
		}
		cfg.applyDefaults()
		errs := []string{}
		if cfg.APIVersion != GeneratorAPIVersion {
			errs = append(errs, fmt.Sprintf("expected apiVersion %s but was %s", GeneratorAPIVersion, cfg.APIVersion))
		}
		if cfg.Kind != GeneratorKind {
			errs = append(errs, fmt.Sprintf("expected kind %s but was %s", GeneratorKind, cfg.Kind))
		}
		if cfg.Metadata.Name == "" {
			errs = append(errs, "metadata.name was not set")
		}
		errs = append(errs, cfg.Validate()...)
		if len(errs) > 0 {
			return nil, errors.Errorf("invalid chart renderer config:\n\t* %s", strings.Join(errs, "\n\t* "))
		}
	}
	return cfg, errors.Wrap(err, "read chart renderer config")
}
