package helm

import (
	"os"

	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
)

// Config specifies the local helm configuration and repository policy
type Config struct {
	Home                string
	Debug               bool
	AcceptAnyRepository *bool
}

// NewConfig creates a helm config with default values
func NewConfig() Config {
	helmHome := os.Getenv("HELM_HOME")
	if helmHome == "" {
		helmHome = environment.DefaultHelmHome
	}
	return Config{Home: helmHome}
}

// Helm maintains the helm environment state
type Helm struct {
	acceptAnyRepository *bool
	Settings            environment.EnvSettings
	Getters             getter.Providers
}

// NewHelm creates a new helm environment
func NewHelm(cfg Config) *Helm {
	h := &Helm{acceptAnyRepository: cfg.AcceptAnyRepository,
		Settings: environment.EnvSettings{
			Home:  helmpath.Home(cfg.Home),
			Debug: cfg.Debug,
		}}
	h.Getters = getter.All(h.Settings)
	return h
}
