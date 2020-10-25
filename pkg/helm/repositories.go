package helm

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/getter"
	cli "k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/repo"
)

type repositoryConfig interface {
	io.Closer
	HelmHome() helmpath.Home
	ResolveChartVersion(name, version, repoURL string) (*repo.ChartVersion, error)
	EntryByURL(repoURL string) (*repo.Entry, error)
	UpdateIndex() error
	DownloadIndexFilesIfNotExist() error
}

func reposForURLs(settings *cli.EnvSettings, getters getter.Providers, repoURLs map[string]struct{}) (*repositories, error) {
	repos, err := newRepositories(settings, getters)
	if err != nil {
		return nil, err
	}
	err = repos.setRepositoriesFromURLs(repoURLs)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// tempRepositoriesWithDependencies create temporary repositories.yaml and configure settings with it.
func tempReposForDependencies(settings *cli.EnvSettings, getters getter.Providers, deps []*chartutil.Dependency) (repositoryConfig, error) {
	repoURLs := map[string]struct{}{}
	for _, d := range deps {
		repoURLs[d.Repository] = struct{}{}
	}
	r, err := reposForURLs(settings, getters, repoURLs)
	if err != nil {
		return nil, err
	}
	repos, err := r.Apply()
	if err != nil {
		return nil, err
	}
	settings.Home = repos.HelmHome()
	return repos, nil
}

type repositories struct {
	dir          helmpath.Home
	repos        *repo.RepoFile
	repoURLMap   map[string]*repo.Entry
	getters      getter.Providers
	cacheDir     string
	entriesAdded bool
	indexFiles   map[string]*repo.IndexFile
}

func newRepositories(settings *cli.EnvSettings, getters getter.Providers) (*repositories, error) {
	repoFile := settings.Home.RepositoryFile()
	repoURLMap := map[string]*repo.Entry{}
	repos, err := repo.LoadRepositoriesFile(repoFile)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, err
		}
		repos = repo.NewRepoFile()
	}
	for _, r := range repos.Repositories {
		repoURLMap[r.URL] = r
	}
	cacheDir := settings.Home.Cache()
	return &repositories{settings.Home, repos, repoURLMap, getters, cacheDir, false, map[string]*repo.IndexFile{}}, nil
}

func (f *repositories) repoIndex(entry *repo.Entry) (*repo.IndexFile, error) {
	idx := f.indexFiles[entry.Name]
	if idx != nil {
		return idx, nil
	}
	idxFile := indexFile(entry, f.cacheDir)
	idx, err := repo.LoadIndexFile(idxFile)
	if err != nil {
		if os.IsNotExist(err) {
			err = downloadIndexFile(entry, f.cacheDir, f.getters)
			if err != nil {
				return nil, err
			}
			idx, err = repo.LoadIndexFile(idxFile)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	f.indexFiles[entry.Name] = idx
	return idx, nil
}

func (f *repositories) clearRepoIndex(entry *repo.Entry) {
	f.indexFiles[entry.Name] = nil
}

func (f *repositories) ResolveChartVersion(name, version, repoURL string) (*repo.ChartVersion, error) {
	entry, err := f.EntryByURL(repoURL)
	if err != nil {
		return nil, err
	}
	idx, err := f.repoIndex(entry)
	if err != nil {
		return nil, err
	}
	errMsg := fmt.Sprintf("chart %q", name)
	if version != "" {
		errMsg = fmt.Sprintf("%s version %q", errMsg, version)
	}
	cv, err := idx.Get(name, version)
	if err != nil {
		// Download latest index file and retry lookup if not found
		err = downloadIndexFile(entry, f.cacheDir, f.getters)
		if err != nil {
			return nil, errors.Wrapf(err, "repo index download after %s not found", errMsg)
		}
		f.clearRepoIndex(entry)
		idx, err := f.repoIndex(entry)
		if err != nil {
			return nil, err
		}
		cv, err = idx.Get(name, version)
		if err != nil {
			return nil, errors.Errorf("%s not found in repository %s", errMsg, entry.URL)
		}
	}

	if len(cv.URLs) == 0 {
		return nil, errors.Errorf("%s has no downloadable URLs", errMsg)
	}
	return cv, nil
}

func (f *repositories) HelmHome() helmpath.Home {
	return f.dir
}

func (f *repositories) Apply() (repositoryConfig, error) {
	if !f.entriesAdded {
		return f, nil // don't create temp repos
	}
	return newTempRepositories(f)
}

func (f *repositories) Close() error {
	return nil
}

func (f *repositories) EntryByURL(repoURL string) (*repo.Entry, error) {
	if entry := f.repoURLMap[repoURL]; entry != nil {
		return entry, nil
	}
	return nil, errors.Errorf("repo URL %q is not registered in repositories.yaml", repoURL)
}

func (f *repositories) addRepositoryURL(repoURL string) (*repo.Entry, error) {
	for _, repo := range f.repos.Repositories {
		f.repoURLMap[repo.URL] = repo
	}
	name, err := urlToHash(repoURL)
	if err != nil {
		return nil, err
	}
	if existing := f.repoURLMap[repoURL]; existing != nil {
		return existing, nil
	}
	entry := &repo.Entry{
		Name: name,
		URL:  repoURL,
	}
	f.repos.Add(entry)
	f.repoURLMap[entry.URL] = entry
	f.entriesAdded = true
	return entry, nil
}

func (f *repositories) DownloadIndexFilesIfNotExist() error {
	for _, r := range f.repos.Repositories {
		if _, err := os.Stat(indexFile(r, f.cacheDir)); err == nil {
			continue // do not update existing repo index
		}
		if err := downloadIndexFile(r, f.cacheDir, f.getters); err != nil {
			return errors.Wrap(err, "download repo index")
		}
	}
	return nil
}

func (f *repositories) UpdateIndex() error {
	for _, r := range f.repos.Repositories {
		if err := downloadIndexFile(r, f.cacheDir, f.getters); err != nil {
			return errors.Wrap(err, "download repo index")
		}
	}
	return nil
}

func (f *repositories) setRepositoriesFromURLs(repoURLMap map[string]struct{}) error {
	requiredRepos := repo.NewRepoFile()
	for _, repo := range f.repos.Repositories {
		if _, ok := repoURLMap[repo.URL]; ok {
			requiredRepos.Add(repo)
			delete(repoURLMap, repo.URL)
		}
	}
	f.repos.Repositories = requiredRepos.Repositories
	repoURLs := make([]string, 0, len(repoURLMap))
	for u := range repoURLMap {
		repoURLs = append(repoURLs, u)
		f.entriesAdded = true
	}
	sort.Strings(repoURLs)
	for _, repoURL := range repoURLs {
		if _, err := f.addRepositoryURL(repoURL); err != nil {
			return err
		}
	}
	return nil
}

type tempRepositories struct {
	*repositories
	tmpDir helmpath.Home
}

func newTempRepositories(r *repositories) (tmp *tempRepositories, err error) {
	tmpDir, err := ioutil.TempDir("", "helm-home-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpDir)
		}
	}()
	for _, dir := range []string{filepath.Join("repository", "cache"), filepath.Join("cache", "archive")} {
		cacheDir := filepath.Join(string(r.dir), dir)
		err = os.MkdirAll(filepath.Dir(cacheDir), 0755)
		if err != nil {
			return nil, err
		}
		tmpCacheLink := filepath.Join(tmpDir, dir)
		err = os.MkdirAll(filepath.Dir(tmpCacheLink), 0755)
		if err != nil {
			return nil, err
		}
		err = os.Symlink(cacheDir, tmpCacheLink)
		if err != nil {
			return nil, err
		}
	}
	tmpHome := helmpath.Home(tmpDir)
	reposFile := tmpHome.RepositoryFile()
	err = r.repos.WriteFile(reposFile, 0640)
	if err != nil {
		return nil, err
	}
	return &tempRepositories{r, tmpHome}, nil
}

func (f *tempRepositories) HelmHome() helmpath.Home {
	return f.tmpDir
}

func (f *tempRepositories) Close() error {
	return os.Remove(string(f.tmpDir))
}

func downloadIndexFile(entry *repo.Entry, cacheDir string, getters getter.Providers) error {
	r, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return err
	}
	log.Printf("Downloading repo index of %s", entry.URL)
	idxFile := indexFile(entry, cacheDir)
	err = os.MkdirAll(filepath.Dir(idxFile), 0755)
	if err != nil {
		return err
	}
	err = r.DownloadIndexFile(idxFile)
	if err != nil {
		return errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", entry.URL)
	}
	return nil
}

func indexFile(entry *repo.Entry, cacheDir string) string {
	return filepath.Join(cacheDir, fmt.Sprintf("%s-index.yaml", entry.Name))
}

func urlToHash(str string) (string, error) {
	hash := crypto.SHA256.New()
	if _, err := io.Copy(hash, bytes.NewReader([]byte(str))); err != nil {
		return "", nil
	}
	name := hex.EncodeToString(hash.Sum(nil))
	return strings.ToLower(strings.TrimRight(name, "=")), nil
}
