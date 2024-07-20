package config

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/client-go/util/homedir"
)

const (
	// GeneratorAPIVersion specifies the apiVersion field value supported by the generator
	GeneratorAPIVersion = "khelm.mgoltzsche.github.com/v2"
	// GeneratorKind specifies the API kind field value supported by the generator
	GeneratorKind = "ChartRenderer"
)

// KRMFuncConfig defines the KRM function input.
type KRMFuncConfigFile struct {
	APIVersion    string      `yaml:"apiVersion"`
	Kind          string      `yaml:"kind"`
	Metadata      K8sMetadata `yaml:"metadata"`
	KRMFuncConfig `yaml:",inline"`
	// Data is specified for backward-compatibility with an early kustomize krm function version.
	// Deprecated.
	Data *KRMFuncConfig `yaml:"data,omitempty"`
}

// KRMFuncConfig defines the KRM function input.
type KRMFuncConfig struct {
	ChartConfig       `yaml:",inline"`
	OutputPath        string                 `yaml:"outputPath,omitempty"`
	OutputPathMapping []KRMFuncOutputMapping `yaml:"outputPathMapping,omitempty"`
	Debug             bool                   `yaml:"debug,omitempty"`
}

// KRMFuncOutputMapping maps resources that match the selector to the specified output path.
type KRMFuncOutputMapping struct {
	Selectors  []ResourceSelector `yaml:"selectors,omitempty"`
	OutputPath string             `yaml:"outputPath"`
}

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
func NewChartConfig() (cfg *ChartConfig) {
	cfg = &ChartConfig{}
	cfg.ApplyDefaults()
	return
}

func (cfg *ChartConfig) ApplyDefaults() {
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	if cfg.KubeVersion == "" {
		cfg.KubeVersion = chartutil.DefaultCapabilities.KubeVersion.Version
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
	Repository      string `yaml:"repository,omitempty"`
	Chart           string `yaml:"chart"`
	Version         string `yaml:"version,omitempty"`
	Verify          bool   `yaml:"verify,omitempty"`
	Keyring         string `yaml:"keyring,omitempty"`
	ReplaceLockFile bool   `yaml:"replaceLockFile,omitempty"`
}

// RendererConfig defines the configuration to render a chart
type RendererConfig struct {
	Name           string                 `yaml:"name,omitempty"`
	Namespace      string                 `yaml:"namespace,omitempty"`
	ValueFiles     []string               `yaml:"valueFiles,omitempty"`
	Values         map[string]interface{} `yaml:"values,omitempty"`
	KubeVersion    string                 `yaml:"kubeVersion,omitempty"`
	APIVersions    []string               `yaml:"apiVersions,omitempty"`
	ExcludeCRDs    bool                   `yaml:"excludeCRDs,omitempty"` // TODO: test this option
	Include        []ResourceSelector     `yaml:"include,omitempty"`
	Exclude        []ResourceSelector     `yaml:"exclude,omitempty"`
	ExcludeHooks   bool                   `yaml:"excludeHooks,omitempty"`
	NamespacedOnly bool                   `yaml:"namespacedOnly,omitempty"`
	ForceNamespace string                 `yaml:"forceNamespace,omitempty"`
}

// ResourceSelector specifies a Kubernetes resource selector
type ResourceSelector struct {
	APIVersion string `yaml:"apiVersion,omitempty"`
	Kind       string `yaml:"kind,omitempty"`
	Namespace  string `yaml:"namespace,omitempty"`
	Name       string `yaml:"name,omitempty"`
}

// Validate validates the chart renderer config
func (cfg *ChartConfig) Validate() (errs []string) {
	if cfg.Chart == "" {
		errs = append(errs, "chart not specified")
	}
	if cfg.Name == "" {
		errs = append(errs, "release name not specified")
	}
	if cfg.Namespace == "" {
		errs = append(errs, "release namespace not specified")
	}
	return
}

// ReadGeneratorConfig read the generator configuration
func ReadGeneratorConfig(reader io.Reader) (cfg *GeneratorConfig, err error) {
	cfg = &GeneratorConfig{}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, errors.Wrap(err, "read chart renderer config")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	err = dec.Decode(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "read chart renderer config")
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
		cfg.ApplyDefaults()
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
			return nil, errors.Errorf("invalid chart renderer config:\n * %s", strings.Join(errs, "\n * "))
		}
	}
	return cfg, errors.Wrap(err, "read chart renderer config")
}
