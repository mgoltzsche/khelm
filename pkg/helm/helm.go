package helm

import (
	"log"
	"os"

	"github.com/pkg/errors"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/repo"
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
	settings            environment.EnvSettings
	getters             getter.Providers
}

// NewHelm creates a new helm environment
func NewHelm(cfg Config) (*Helm, error) {
	h := &Helm{acceptAnyRepository: cfg.AcceptAnyRepository,
		settings: environment.EnvSettings{
			Home:  helmpath.Home(cfg.Home),
			Debug: cfg.Debug,
		}}
	h.getters = getter.All(h.settings)
	return h, initializeHelmHome(h.settings.Home)
}

// initializeHelmHome initialize the helm home directory.
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/installer/init.go
func initializeHelmHome(home helmpath.Home) (err error) {
	// Create directories
	for _, dir := range []string{
		home.String(),
		home.Repository(),
		home.Cache(),
		home.LocalRepository(),
		home.Plugins(),
		home.Starters(),
		home.Archive(),
	} {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return errors.WithStack(err)
		}
	}

	// Create repo file
	if _, err = os.Stat(home.RepositoryFile()); err != nil && os.IsNotExist(err) {
		log.Printf("Initializing empty %s", home.RepositoryFile())
		f := repo.NewRepoFile()
		err = f.WriteFile(home.RepositoryFile(), 0644)
	}
	return errors.WithStack(err)
}
