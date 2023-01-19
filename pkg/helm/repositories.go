package helm

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/repo"
)

type repositoryConfig interface {
	Close() error
	File() string
	ResolveChartVersion(ctx context.Context, name, version, repo string) (*repo.ChartVersion, error)
	Get(repo string) (*repo.Entry, bool, error)
	UpdateIndex(context.Context) error
	FetchMissingIndexFiles(context.Context) error
	TempRepositories() (repositoryConfig, error)
}

func reposForURLs(repoURLs map[string]struct{}, repos repositories.Interface) (repositoryConfig, error) {
	repoFile, untrusted, err := repoFileFromURLs(repoURLs, repos)
	if err != nil {
		return nil, err
	}
	r := &requiredRepos{
		Interface: repos,
		repoFile:  repoFile,
	}
	if untrusted {
		return newTempRepositories(r)
	}
	return r, nil
}

// reposForDependencies create temporary repositories.yaml and configure settings with it.
func reposForDependencies(deps []*chart.Dependency, repos repositories.Interface) (repositoryConfig, error) {
	repoURLs := map[string]struct{}{}
	for _, d := range deps {
		repoURLs[d.Repository] = struct{}{}
	}
	return reposForURLs(repoURLs, repos)
}

type requiredRepos struct {
	repositories.Interface
	repoFile *repo.File
}

func (r *requiredRepos) TempRepositories() (repositoryConfig, error) {
	tr, err := newTempRepositories(r)
	if err != nil {
		return nil, fmt.Errorf("new temp repositories: %w", err)
	}
	return tr, nil
}

func (_ *requiredRepos) Close() error {
	return nil
}

func (r *requiredRepos) FetchMissingIndexFiles(ctx context.Context) error {
	for _, entry := range r.repoFile.Repositories {
		err := r.Interface.FetchIndexIfNotExist(ctx, entry)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *requiredRepos) UpdateIndex(ctx context.Context) error {
	for _, entry := range r.repoFile.Repositories {
		err := r.Interface.UpdateIndex(ctx, entry)
		if err != nil {
			return err
		}
	}
	return nil
}

func repoFileFromURLs(repoURLSet map[string]struct{}, repos repositories.Interface) (*repo.File, bool, error) {
	repoURLMap := make(map[string]*repo.Entry, len(repoURLSet))
	repoURLs := make([]string, 0, len(repoURLMap))
	for k := range repoURLSet {
		repoURLs = append(repoURLs, k)
	}
	sort.Strings(repoURLs)
	untrusted := false
	newRepos := repo.NewFile()
	for _, u := range repoURLs {
		entry, found, err := repos.Get(u)
		if err != nil {
			return nil, false, err
		}
		newRepos.Add(entry)
		if found {
			authInfo := "unauthenticated"
			if entry.Username != "" && entry.Password != "" {
				authInfo = fmt.Sprintf("as user %q", entry.Username)
			}
			log.Printf("Using repository %q (%s)", u, authInfo)
		} else {
			untrusted = true
			log.Printf("WARNING: using untrusted repository %q", u)
		}
	}
	return newRepos, untrusted, nil
}

type tempRepositories struct {
	*requiredRepos
	tmpFile string
}

func newTempRepositories(r *requiredRepos) (*tempRepositories, error) {
	tmpFile, err := os.CreateTemp("", "helm-repositories-")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	_ = tmpFile.Close()
	err = r.repoFile.WriteFile(tmpFile.Name(), 0640)
	return &tempRepositories{r, tmpFile.Name()}, err
}

func (r *tempRepositories) File() string {
	return r.tmpFile
}

func (r *tempRepositories) Close() error {
	return os.Remove(r.tmpFile)
}

func (r *tempRepositories) TempRepositories() (repositoryConfig, error) {
	return r, nil
}
