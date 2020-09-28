package helm

import (
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
)

// Helm maintains the helm environment state
type Helm struct {
	TrustAnyRepository *bool
	Settings           cli.EnvSettings
	Getters            getter.Providers
}

// NewHelm creates a new helm environment
func NewHelm() *Helm {
	settings := cli.New()
	if helmHome := os.Getenv("HELM_HOME"); helmHome != "" && os.Getenv(helmpath.ConfigHomeEnvVar) == "" {
		// Fallback for old helm env var
		settings.RepositoryConfig = filepath.Join(helmHome, "repository", "repositories.yaml")
	}
	return &Helm{Settings: *settings, Getters: getter.All(settings)}
}
