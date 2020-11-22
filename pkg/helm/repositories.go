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

type unknownRepoError struct {
	error
}

func (e *unknownRepoError) Format(s fmt.State, verb rune) {
	f, isFormatter := e.error.(interface {
		Format(s fmt.State, verb rune)
	})
	if isFormatter {
		f.Format(s, verb)
		return
	}
	fmt.Fprintf(s, "%s", e.error)
}

// IsUnknownRepository return true if the provided error is an unknown repository error
func IsUnknownRepository(err error) bool {
	_, ok := errors.Cause(err).(*unknownRepoError)
	return ok
}

type repositoryConfig interface {
	Close() error
	HelmHome() helmpath.Home
	ResolveChartVersion(name, version, repo string) (*repo.ChartVersion, error)
	Get(repo string) (*repo.Entry, error)
	UpdateIndex() error
	DownloadIndexFilesIfNotExist() error
	Apply() (repositoryConfig, error)
}

func reposForURLs(repoURLs map[string]struct{}, trustAnyRepo *bool, settings *cli.EnvSettings, getters getter.Providers) (repositoryConfig, error) {
	repos, err := newRepositories(settings, getters)
	if err != nil {
		return nil, err
	}
	err = repos.setRepositoriesFromURLs(repoURLs, trustAnyRepo)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// reposForDependencies create temporary repositories.yaml and configure settings with it.
func reposForDependencies(deps []*chartutil.Dependency, trustAnyRepo *bool, settings *cli.EnvSettings, getters getter.Providers) (repositoryConfig, error) {
	repoURLs := map[string]struct{}{}
	for _, d := range deps {
		repoURLs[d.Repository] = struct{}{}
	}
	repos, err := reposForURLs(repoURLs, trustAnyRepo, settings, getters)
	if err != nil {
		return nil, err
	}
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

func newRepositories(settings *cli.EnvSettings, getters getter.Providers) (r *repositories, err error) {
	r = &repositories{
		dir:        settings.Home,
		repoURLMap: map[string]*repo.Entry{},
		getters:    getters,
		cacheDir:   settings.Home.Cache(),
		indexFiles: map[string]*repo.IndexFile{},
	}
	if !filepath.IsAbs(string(settings.Home)) {
		return nil, errors.Errorf("helm home must specify absolute file path but was %q", settings.Home)
	}
	repoFile := settings.Home.RepositoryFile()
	r.repos, err = repo.LoadRepositoriesFile(repoFile)
	if err != nil {
		if _, e := os.Stat(repoFile); e != nil && !os.IsNotExist(e) {
			return nil, errors.Wrapf(err, "load %s", repoFile)
		}
	} else {
		for _, e := range r.repos.Repositories {
			r.repoURLMap[e.URL] = e
		}
	}
	if err = initializeHelmHome(settings.Home); err != nil {
		return nil, errors.Wrap(err, "initialize helm home")
	}
	return r, nil
}

// initializeHelmHome initialize the helm home directory without repositories.yaml.
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
	return nil
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
				return nil, errors.WithStack(err)
			}
		} else {
			return nil, errors.WithStack(err)
		}
	}
	f.indexFiles[entry.Name] = idx
	return idx, nil
}

func (f *repositories) clearRepoIndex(entry *repo.Entry) {
	f.indexFiles[entry.Name] = nil
}

func (f *repositories) ResolveChartVersion(name, version, repoURL string) (*repo.ChartVersion, error) {
	entry, err := f.Get(repoURL)
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

func (f *repositories) Get(repo string) (*repo.Entry, error) {
	isName := false
	if strings.HasPrefix(repo, "alias:") {
		repo = repo[6:]
		isName = true
	}
	if strings.HasPrefix(repo, "@") {
		repo = repo[1:]
		isName = true
	}
	if isName {
		if entry, _ := f.repos.Get(repo); entry != nil {
			return entry, nil
		}
		return nil, errors.Errorf("repo name %q is not registered in repositories.yaml", repo)
	}
	if entry := f.repoURLMap[repo]; entry != nil {
		return entry, nil
	}
	return nil, errors.Errorf("repo URL %q is not registered in repositories.yaml", repo)
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

func (f *repositories) setRepositoriesFromURLs(repoURLs map[string]struct{}, trustAnyRepo *bool) error {
	requiredRepos := make([]*repo.Entry, 0, len(repoURLs))
	repoURLMap := map[string]*repo.Entry{}
	for u := range repoURLs {
		repo, _ := f.Get(u)
		if repo != nil {
			u = repo.URL
		} else if strings.HasPrefix(u, "alias:") || strings.HasPrefix(u, "@") {
			return errors.Errorf("repository %q not found in repositories.yaml", u)
		} else if trustAnyRepo != nil && !*trustAnyRepo || trustAnyRepo == nil && f.repos != nil {
			err := errors.Errorf("repository %q not found in repositories.yaml and usage of unknown repositories is disabled", u)
			if f.repos == nil {
				err = errors.Errorf("request repository %q: %s does not exist and usage of unknown repositories is disabled", u, f.dir.RepositoryFile())
			}
			return &unknownRepoError{err}
		}
		repoURLMap[u] = repo
	}
	if f.repos != nil {
		for _, entry := range f.repos.Repositories {
			if repo := repoURLMap[entry.URL]; repo != nil {
				requiredRepos = append(requiredRepos, repo)
			}
		}
	}
	f.repos = repo.NewRepoFile()
	f.repos.Repositories = requiredRepos
	newURLs := make([]string, 0, len(repoURLMap))
	for u, knownRepo := range repoURLMap {
		if knownRepo == nil {
			newURLs = append(newURLs, u)
			f.entriesAdded = true
		}
	}
	sort.Strings(newURLs)
	for _, repoURL := range newURLs {
		if _, err := f.addRepositoryURL(repoURL); err != nil {
			return err
		}
	}

	// Log repository usage
	repoUsage := make([]string, len(f.repos.Repositories))
	for i, entry := range f.repos.Repositories {
		if repo := repoURLMap[entry.URL]; repo != nil || trustAnyRepo != nil {
			authInfo := "unauthenticated"
			if entry.Username != "" && entry.Password != "" {
				authInfo = fmt.Sprintf("as user %q", entry.Username)
			}
			repoUsage[i] = fmt.Sprintf("Using repository %q (%s)", entry.URL, authInfo)
		} else {
			repoUsage[i] = fmt.Sprintf("WARNING: using untrusted repository %q", entry.URL)
		}
	}
	sort.Strings(repoUsage)
	for _, msg := range repoUsage {
		log.Println(msg)
	}

	return nil
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
	entry.Cache = indexFile(entry, f.cacheDir)
	f.repos.Add(entry)
	f.repoURLMap[entry.URL] = entry
	f.entriesAdded = true
	return entry, nil
}

type tempRepositories struct {
	*repositories
	tmpDir helmpath.Home
}

func newTempRepositories(r *repositories) (tmp *tempRepositories, err error) {
	tmpDir, err := ioutil.TempDir("", "helm-home-")
	if err != nil {
		return nil, errors.WithStack(err)
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
			return nil, errors.WithStack(err)
		}
		tmpCacheLink := filepath.Join(tmpDir, dir)
		err = os.MkdirAll(filepath.Dir(tmpCacheLink), 0755)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		err = os.Symlink(cacheDir, tmpCacheLink)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	tmpHome := helmpath.Home(tmpDir)
	reposFile := tmpHome.RepositoryFile()
	err = r.repos.WriteFile(reposFile, 0640)
	if err != nil {
		return nil, errors.WithStack(err)
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
	log.Printf("Downloading repository index of %s", entry.URL)
	r, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return errors.WithStack(err)
	}
	idxFile := entry.Cache
	if idxFile == "" {
		idxFile = indexFile(entry, cacheDir)
	}
	err = os.MkdirAll(filepath.Dir(idxFile), 0755)
	if err != nil {
		return errors.WithStack(err)
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
		return "", errors.Wrap(err, "urlToHash")
	}
	name := hex.EncodeToString(hash.Sum(nil))
	return strings.ToLower(strings.TrimRight(name, "=")), nil
}
