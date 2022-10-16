package helm

import (
	"os"
	"path/filepath"

	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
)

// Helm maintains the helm environment state
type Helm struct {
	TrustAnyRepository *bool
	Settings           cli.EnvSettings
	Getters            getter.Providers
	repos              repositories.Interface
}

// NewHelm creates a new helm environment
func NewHelm() *Helm {
	settings := cli.New()
	if helmHome := os.Getenv("HELM_HOME"); helmHome != "" && os.Getenv(helmpath.ConfigHomeEnvVar) == "" {
		// Fallback for old helm env var
		settings.RepositoryConfig = filepath.Join(helmHome, "repository", "repositories.yaml")
	}
	h := &Helm{Settings: *settings}
	h.Getters = getters(settings, h.repositories)
	return h
}

func (h *Helm) repositories() (repositories.Interface, error) {
	if h.repos != nil {
		return h.repos, nil
	}
	repos, err := repositories.New(h.Settings, h.Getters, h.TrustAnyRepository)
	if err != nil {
		return nil, err
	}
	h.repos = repos
	return repos, nil
}
