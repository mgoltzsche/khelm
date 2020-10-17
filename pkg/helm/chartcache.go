package helm

import (
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// TODO: finish impl or remove

type chartCache struct {
	cacheDir string
}

func (c *chartCache) AddDependenciesFromCache(destChart *chart.Chart) error {
	/*for _, dep := range remoteDependencies(destChart) {

	}*/
	return nil
}

func (c *chartCache) AddDependenciesToCache(srcChart *chart.Chart) error {
	return nil
}

func remoteDependencies(c *chart.Chart) (d []*chart.Dependency) {
	deps := c.Metadata.Dependencies
	if c.Lock != nil {
		deps = c.Lock.Dependencies
	}
	for _, dep := range deps {
		if strings.HasPrefix(dep.Repository, "https://") || strings.HasPrefix(dep.Repository, "http://") {
			d = append(d, dep)
		}
	}
	return
}
