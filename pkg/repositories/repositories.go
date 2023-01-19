package repositories

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/repo"
)

type untrustedRepoError struct {
	error
}

func IsUntrustedRepository(err error) bool {
	var urErr *untrustedRepoError
	return errors.As(err, &urErr)
}

type Interface interface {
	File() string
	ResolveChartVersion(ctx context.Context, name, version, repoURL string) (*repo.ChartVersion, error)
	Get(repo string) (*repo.Entry, bool, error)
	FetchIndexIfNotExist(ctx context.Context, entry *repo.Entry) error
	UpdateIndex(ctx context.Context, entry *repo.Entry) error
}

type repositories struct {
	file         string
	repos        *repo.File
	repoURLMap   map[string]*repo.Entry
	getters      getter.Providers
	cacheDir     string
	indexFiles   map[string]*repo.IndexFile
	trustAnyRepo *bool
}

func New(settings cli.EnvSettings, getters getter.Providers, trustAnyRepo *bool) (Interface, error) {
	if !filepath.IsAbs(settings.RepositoryConfig) {
		return nil, fmt.Errorf("path to repositories.yaml must be absolute but was %q", settings.RepositoryConfig)
	}
	repoURLMap := map[string]*repo.Entry{}
	repos, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		if !os.IsNotExist(errors.Unwrap(errors.Unwrap(err))) {
			return nil, fmt.Errorf("load repo config: %w", err)
		}
		repos = nil
	} else {
		for _, e := range repos.Repositories {
			repoURLMap[e.URL] = e
		}
	}
	if err = os.MkdirAll(settings.RepositoryCache, 0750); err != nil {
		return nil, fmt.Errorf("create repo cache dir: %w", err)
	}
	return &repositories{
		file:         settings.RepositoryConfig,
		repos:        repos,
		repoURLMap:   repoURLMap,
		getters:      getters,
		cacheDir:     settings.RepositoryCache,
		indexFiles:   map[string]*repo.IndexFile{},
		trustAnyRepo: trustAnyRepo,
	}, nil
}

func (r *repositories) File() string {
	return r.file
}

func (r *repositories) ResolveChartVersion(ctx context.Context, name, version, repoURL string) (*repo.ChartVersion, error) {
	entry, _, err := r.Get(repoURL)
	if err != nil {
		return nil, err
	}
	idx, err := r.index(ctx, entry)
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
		idx, err := r.update(ctx, entry)
		if err != nil {
			return nil, fmt.Errorf("%s not found in repository %s: %w", errMsg, entry.URL, err)
		}
		cv, err = idx.Get(name, version)
		if err != nil {
			return nil, fmt.Errorf("%s not found in repository %s", errMsg, entry.URL)
		}
	}
	if len(cv.URLs) == 0 {
		return nil, fmt.Errorf("%s has no downloadable URLs", errMsg)
	}
	return cv, nil
}

func (r *repositories) Get(repo string) (*repo.Entry, bool, error) {
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
		if r.repos == nil {
			return nil, false, fmt.Errorf("resolve repo name %q: no repositories.yaml configured to resolve repo alias with", repo)
		}
		if e := r.repos.Get(repo); e != nil {
			entry := *e
			return &entry, true, nil
		}
		return nil, false, fmt.Errorf("repo name %q is not registered in repositories.yaml", repo)
	}
	if entry := r.repoURLMap[repo]; entry != nil {
		return entry, true, nil
	}
	if r.trustAnyRepo != nil && !*r.trustAnyRepo || r.trustAnyRepo == nil && r.repos != nil {
		err := fmt.Errorf("repository %q not found in %s and usage of untrusted repositories is disabled", repo, r.file)
		if r.repos == nil {
			err = fmt.Errorf("use repository %s: configuration %s does not exist and usage of untrusted repositories is disabled", repo, r.file)
		}
		return nil, false, &untrustedRepoError{err}
	}
	return newEntry(repo), false, nil
}

func (r *repositories) FetchIndexIfNotExist(ctx context.Context, entry *repo.Entry) error {
	_, err := r.index(ctx, entry)
	return err
}

func (r *repositories) index(ctx context.Context, entry *repo.Entry) (*repo.IndexFile, error) {
	if idx := r.indexFiles[entry.Name]; idx != nil {
		return idx, nil
	}
	idxFile := indexFile(entry, r.cacheDir)
	idx, err := loadIndexFile(ctx, idxFile)
	if err != nil {
		if os.IsNotExist(errors.Unwrap(err)) {
			return r.update(ctx, entry)
		}
	}
	return idx, err
}

func (r *repositories) UpdateIndex(ctx context.Context, entry *repo.Entry) error {
	_, err := r.update(ctx, entry)
	return err
}

func (r *repositories) update(ctx context.Context, entry *repo.Entry) (*repo.IndexFile, error) {
	err := downloadIndexFile(ctx, entry, r.cacheDir, r.getters)
	if err != nil {
		return nil, fmt.Errorf("download index for repo %s: %w", entry.URL, err)
	}
	idxFile := indexFile(entry, r.cacheDir)
	idx, err := loadIndexFile(ctx, idxFile)
	if err != nil {
		return nil, err
	}
	r.indexFiles[entry.Name] = idx
	return idx, nil
}

func newEntry(repoURL string) *repo.Entry {
	name, err := urlToHash(repoURL)
	if err != nil {
		panic(fmt.Errorf("hash repo url: %w", err))
	}
	return &repo.Entry{
		Name: name,
		URL:  repoURL,
	}
}

func urlToHash(str string) (string, error) {
	hash := crypto.SHA256.New()
	if _, err := io.Copy(hash, bytes.NewReader([]byte(str))); err != nil {
		return "", err
	}
	name := hex.EncodeToString(hash.Sum(nil))
	return strings.ToLower(strings.TrimRight(name, "=")), nil
}

func loadIndexFile(ctx context.Context, idxFile string) (idx *repo.IndexFile, err error) {
	done := make(chan struct{}, 1)
	go func() {
		idx, err = repo.LoadIndexFile(idxFile)
		close(done)
	}()
	select {
	case <-done:
		if err != nil {
			return nil, fmt.Errorf("load repo index file %s: %w", idxFile, err)
		}
		return idx, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("load repo index file %s: %w", idxFile, ctx.Err())
	}
}

func downloadIndexFile(ctx context.Context, entry *repo.Entry, cacheDir string, getters getter.Providers) error {
	log.Printf("Downloading repository index of %s", entry.URL)
	idxFile := indexFile(entry, cacheDir)
	err := os.MkdirAll(filepath.Dir(idxFile), 0750)
	if err != nil {
		return err
	}
	tmpIdxFile, err := os.CreateTemp(filepath.Dir(idxFile), fmt.Sprintf(".tmp-%s-index", entry.Name))
	if err != nil {
		return err
	}
	tmpIdxFileName := tmpIdxFile.Name()
	_ = tmpIdxFile.Close()

	interrupt := ctx.Done()
	done := make(chan error, 1)
	go func() {
		var err error
		defer func() {
			done <- err
			close(done)
			if err != nil {
				_ = os.Remove(tmpIdxFileName)
			}
		}()
		tmpEntry := *entry
		tmpEntry.Name = filepath.Base(tmpIdxFileName)
		r, e := repo.NewChartRepository(&tmpEntry, getters)
		if e != nil {
			err = e
			return
		}
		r.CachePath = filepath.Dir(tmpIdxFileName)
		tmpIdxFileName, err = r.DownloadIndexFile()
		if err != nil {
			err = fmt.Errorf("looks like %q is not a valid chart repository or cannot be reached: %w", entry.URL, err)
			return
		}
		err = os.Rename(tmpIdxFileName, idxFile)
		if os.IsExist(err) {
			err = os.Remove(tmpIdxFileName)
		}
	}()
	select {
	case err := <-done:
		return err
	case <-interrupt:
		_ = os.Remove(tmpIdxFileName)
		return ctx.Err()
	}
}

func indexFile(entry *repo.Entry, cacheDir string) string {
	return filepath.Join(cacheDir, helmpath.CacheIndexFile(entry.Name))
}
