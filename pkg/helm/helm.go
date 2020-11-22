package helm

import (
	"os"

	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
)

// Helm maintains the helm environment state
type Helm struct {
	TrustAnyRepository *bool
	Settings           environment.EnvSettings
	Getters            getter.Providers
}

// NewHelm creates a new helm environment
func NewHelm() *Helm {
	helmHome := os.Getenv("HELM_HOME")
	if helmHome == "" {
		helmHome = environment.DefaultHelmHome
	}
	h := &Helm{Settings: environment.EnvSettings{
		Home: helmpath.Home(helmHome),
	}}
	h.Getters = getter.All(h.Settings)
	return h
}
