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
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/repo"
)

type repositoryConfig interface {
	io.Closer
	FilePath() string
	DownloadIndexFiles() error
	ResolveChartVersion(name, version, repoURL string) (*repo.ChartVersion, error)
	EntryByURL(repoURL string) (*repo.Entry, error)
}

func useRepo(repoURL string, settings *cli.EnvSettings, getters getter.Providers) (r *repo.Entry, repos repositoryConfig, err error) {
	origRepos, err := newRepositories(settings, getters)
	if err != nil {
		return nil, nil, err
	}
	r, err = origRepos.addRepositoryURL(repoURL)
	if err != nil {
		return nil, nil, err
	}
	repos, err = origRepos.Apply()
	if err != nil {
		return nil, nil, err
	}
	err = repos.DownloadIndexFiles()
	if err != nil {
		repos.Close()
		return nil, nil, err
	}
	settings.RepositoryConfig = repos.FilePath()
	return
}

// tempRepositoriesWithDependencies create temporary repositories.yaml and configure settings with it.
func tempRepositoriesWithDependencies(settings *cli.EnvSettings, getters getter.Providers, repoURLs map[string]struct{}) (repositoryConfig, error) {
	r, err := newRepositories(settings, getters)
	if err != nil {
		return nil, err
	}
	err = r.setRepositoriesFromURLs(repoURLs)
	if err != nil {
		return nil, err
	}
	repos, err := r.Apply()
	if err != nil {
		return nil, err
	}
	err = repos.DownloadIndexFiles()
	if err != nil {
		repos.Close()
		return nil, err
	}
	settings.RepositoryConfig = repos.FilePath()
	return repos, nil
}

type repositories struct {
	filePath     string
	repos        *repo.File
	repoURLMap   map[string]*repo.Entry
	getters      getter.Providers
	cacheDir     string
	entriesAdded bool
}

func newRepositories(settings *cli.EnvSettings, getters getter.Providers) (*repositories, error) {
	repoURLMap := map[string]*repo.Entry{}
	repos, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		if !os.IsNotExist(errors.Cause(err)) {
			return nil, err
		}
		repos = repo.NewFile()
	}
	return &repositories{settings.RepositoryConfig, repos, repoURLMap, getters, settings.RepositoryCache, false}, nil
}

func (f *repositories) ResolveChartVersion(name, version, repoURL string) (*repo.ChartVersion, error) {
	entry, err := f.EntryByURL(repoURL)
	if err != nil {
		return nil, err
	}

	idxFile := indexFile(entry, f.cacheDir)
	idx, err := repo.LoadIndexFile(idxFile)
	if err != nil {
		return nil, err
	}
	errMsg := fmt.Sprintf("chart %q", name)
	if version != "" {
		errMsg = fmt.Sprintf("%s version %q", errMsg, version)
	}
	cv, err := idx.Get(name, version)
	if err != nil {
		// Download latest index file again and retry on failure}
		err = downloadIndexFile(entry, f.cacheDir, f.getters)
		if err != nil {
			return nil, errors.Wrapf(err, "repo index download after %s not found", errMsg)
		}
		idx, err := repo.LoadIndexFile(idxFile)
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

func (f *repositories) FilePath() string {
	return f.filePath
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
	return nil, fmt.Errorf("repo URL %q is not registered in repositories.yaml", repoURL)
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

func (f *repositories) DownloadIndexFiles() error {
	return downloadIndexFilesIfMissing(f.repos.Repositories, f.cacheDir, f.getters)
}

func (f *repositories) setRepositoriesFromURLs(urls map[string]struct{}) error {
	requiredRepos := repo.NewFile()
	for _, repo := range f.repos.Repositories {
		if _, ok := urls[repo.URL]; ok {
			requiredRepos.Add(repo)
			delete(urls, repo.URL)
		}
	}
	f.repos.Repositories = requiredRepos.Repositories
	repoURLs := make([]string, 0, len(urls))
	for u := range urls {
		f.entriesAdded = true
		repoURLs = append(repoURLs, u)
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
	tmpFile string
}

func newTempRepositories(r *repositories) (*tempRepositories, error) {
	tmpFile, err := ioutil.TempFile("", "helm-repositories-")
	if err != nil {
		return nil, err
	}
	tmpFile.Close()
	err = r.repos.WriteFile(tmpFile.Name(), 640)
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, err
	}
	return &tempRepositories{r, tmpFile.Name()}, nil
}

func (f *tempRepositories) FilePath() string {
	return f.tmpFile
}

func (f *tempRepositories) Close() error {
	return os.Remove(f.tmpFile)
}

func downloadIndexFilesIfMissing(repos []*repo.Entry, cacheDir string, getters getter.Providers) error {
	for _, r := range repos {
		chartsFile := indexFile(r, cacheDir)
		if _, err := os.Stat(chartsFile); err == nil {
			// TODO: make sure repo indices of dependencies are updated as well when dependency could not be resolved
			continue // do not update existing repo index
		}
		if err := downloadIndexFile(r, cacheDir, getters); err != nil {
			return fmt.Errorf("download repo index: %w", err)
		}
	}
	return nil
}

func downloadIndexFile(entry *repo.Entry, cacheDir string, getters getter.Providers) error {
	r, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return err
	}
	r.CachePath = cacheDir

	log.Printf("Downloading repo index of %s", entry.URL)
	_, err = r.DownloadIndexFile()
	if err != nil {
		return fmt.Errorf("looks like %q is not a valid chart repository or cannot be reached: %w", entry.URL, err)
	}
	return nil
}

func indexFile(entry *repo.Entry, cacheDir string) string {
	return filepath.Join(cacheDir, helmpath.CacheIndexFile(entry.Name))
}

func urlToHash(str string) (string, error) {
	hash := crypto.SHA256.New()
	if _, err := io.Copy(hash, bytes.NewReader([]byte(str))); err != nil {
		return "", nil
	}
	name := hex.EncodeToString(hash.Sum(nil))
	return strings.ToLower(strings.TrimRight(name, "=")), nil
}
