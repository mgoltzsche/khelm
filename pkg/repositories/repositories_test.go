package repositories

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

func init() {
	log.SetFlags(0)
}

func TestRepositories(t *testing.T) {
	dir, err := os.MkdirTemp("", "khelm-test-repositories-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	repoURL := "fake://example.org/myorg/myrepo@some/path?ref=v1.2.3"
	repoConfPath := filepath.Join(dir, "repositories.yaml")
	repoConf := repo.NewFile()
	repoConf.Add(&repo.Entry{
		Name:     "fake-repo",
		URL:      repoURL,
		Username: "fake-user",
		Password: "fake-password",
	})
	b, err := yaml.Marshal(repoConf)
	require.NoError(t, err)
	err = os.WriteFile(repoConfPath, b, 0600)
	require.NoError(t, err)
	settings := cli.New()
	settings.RepositoryCache = filepath.Join(dir, "cache")
	settings.RepositoryConfig = "/fake/non-existing/repositories.yaml"
	chartURL := "fake://example.org/myorg/myrepo@some/path/mychart.tgz?ref=v1.2.3"
	g := &fakeGetter{
		indexURL: "fake://example.org/myorg/myrepo@some/path/index.yaml?ref=v1.2.3",
		chartURL: chartURL,
		index:    fakeIndexFile(chartURL),
	}
	trust := true
	testee := newTestee(t, settings, &trust, g)
	t.Run("File should return repositories config path", func(t *testing.T) {
		file := testee.File()
		require.Equal(t, settings.RepositoryConfig, file, "File()")
		_, err = os.Stat(file)
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})
	t.Run("Get should resolve untrusted repo when trust-any enabled", func(t *testing.T) {
		entry, trusted, err := testee.Get(repoURL)
		require.NoError(t, err, "Get()")
		require.NotNil(t, entry, "entry returned by Get()")
		require.False(t, trusted, "trusted")
	})
	t.Run("Get should not resolve untrusted repo", func(t *testing.T) {
		newSettings := *settings
		newSettings.RepositoryConfig = repoConfPath
		trust := false
		testee := newTestee(t, &newSettings, &trust, g)
		entry, trusted, err := testee.Get(repoURL + "-sth")
		require.Error(t, err, "Get()")
		require.Truef(t, IsUntrustedRepository(err), "error should indicate the repo is untrusted but was: %s", err)
		require.False(t, trusted, "trusted")
		require.Nil(t, entry, "entry returned by Get()")
	})
	var entry *repo.Entry
	t.Run("Get should return credentials for trusted repo", func(t *testing.T) {
		newSettings := *settings
		newSettings.RepositoryConfig = repoConfPath
		trust := false
		testee := newTestee(t, &newSettings, &trust, g)
		var err error
		var trusted bool
		entry, trusted, err = testee.Get(repoURL)
		require.NoError(t, err, "Get()")
		require.NotNil(t, entry, "entry returned by Get()")
		require.True(t, trusted, "trusted")
		require.Equal(t, "fake-user", entry.Username, "username")
		require.Equal(t, "fake-password", entry.Password, "password")
	})
	t.Run("FetchIndexIfNotExist should fetch when file does not exist", func(t *testing.T) {
		err = testee.FetchIndexIfNotExist(context.Background(), entry)
		require.NoError(t, err, "FetchIndexIfNotExist()")
		idxFilePath := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(entry.Name))
		b, err := os.ReadFile(idxFilePath)
		require.NoError(t, err, "read downloaded repo index file")
		idx := repo.NewIndexFile()
		err = yaml.Unmarshal(b, idx)
		require.NoError(t, err, "unmarshal repo index file")
		require.Equal(t, g.index, idx, "repo index")
	})
	t.Run("FetchIndexIfNotExist should not fetch when file exists", func(t *testing.T) {
		origIdx := g.index
		g.index = fakeIndexFile(chartURL)
		g.index.Entries["fakechart"][0].Version = "0.1.1"
		err = testee.FetchIndexIfNotExist(context.Background(), entry)
		require.NoError(t, err, "FetchIndexIfNotExist()")
		idxFilePath := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(entry.Name))
		b, err := os.ReadFile(idxFilePath)
		require.NoError(t, err, "read downloaded repo index file")
		idx := repo.NewIndexFile()
		err = yaml.Unmarshal(b, idx)
		require.NoError(t, err, "unmarshal repo index file")
		require.Equal(t, origIdx, idx, "repo index")
	})
	t.Run("FetchIndexIfNotExist should fail when index file does not exist", func(t *testing.T) {
		newEntry := *entry
		newEntry.Name += "-unknown"
		newEntry.URL += "-unknown"
		err := testee.FetchIndexIfNotExist(context.Background(), &newEntry)
		require.Error(t, err)
	})
	t.Run("UpdateIndex should update index if file exists", func(t *testing.T) {
		g.index.Entries["fakechart"][0].Version = "0.2.0"
		err = testee.UpdateIndex(context.Background(), entry)
		require.NoError(t, err, "UpdateIndex()")
		idxFilePath := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(entry.Name))
		b, err := os.ReadFile(idxFilePath)
		require.NoError(t, err, "read downloaded repo index file")
		idx := repo.NewIndexFile()
		err = yaml.Unmarshal(b, idx)
		require.NoError(t, err, "unmarshal repo index file")
		require.Equal(t, g.index, idx, "repo index")
	})
	t.Run("UpdateIndex should update index if file does not exist", func(t *testing.T) {
		err := os.Remove(filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(entry.Name)))
		require.NoError(t, err, "remove previously downloaded repo index file")
		err = testee.UpdateIndex(context.Background(), entry)
		require.NoError(t, err, "UpdateIndex()")
		idxFilePath := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(entry.Name))
		b, err := os.ReadFile(idxFilePath)
		require.NoError(t, err, "read downloaded repo index file")
		idx := repo.NewIndexFile()
		err = yaml.Unmarshal(b, idx)
		require.NoError(t, err, "unmarshal repo index file")
		require.Equal(t, g.index, idx, "repo index")
	})
	t.Run("ResolveChart should resolve chart version", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		chartVersion, err := testee.ResolveChartVersion(context.Background(), "fakechart", "0.1.0", repoURL)
		require.NoError(t, err)
		require.NotNil(t, chartVersion)
		require.Equal(t, "fakechart", chartVersion.Name, "name")
		require.Equal(t, "0.1.0", chartVersion.Version, "version")
		require.Equal(t, []string{chartURL}, chartVersion.URLs, "urls")
	})
	t.Run("ResolveChart should resolve chart version range", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		chartVersion, err := testee.ResolveChartVersion(context.Background(), "fakechart", "0.x.x", repoURL)
		require.NoError(t, err)
		require.NotNil(t, chartVersion)
		require.Equal(t, "fakechart", chartVersion.Name, "name")
		require.Equal(t, "0.1.0", chartVersion.Version, "version")
		require.Equal(t, []string{chartURL}, chartVersion.URLs, "urls")
	})
	t.Run("ResolveChart should resolve empty chart version", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		chartVersion, err := testee.ResolveChartVersion(context.Background(), "fakechart", "", repoURL)
		require.NoError(t, err)
		require.NotNil(t, chartVersion)
		require.Equal(t, "fakechart", chartVersion.Name, "name")
		require.Equal(t, "0.1.0", chartVersion.Version, "version")
		require.Equal(t, []string{chartURL}, chartVersion.URLs, "urls")
	})
	t.Run("ResolveChart should resolve repo alias", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		newSettings := *settings
		newSettings.RepositoryConfig = repoConfPath
		trust := false
		testee := newTestee(t, &newSettings, &trust, g)
		chartVersion, err := testee.ResolveChartVersion(context.Background(), "fakechart", "0.1.0", "alias:fake-repo")
		require.NoError(t, err)
		require.NotNil(t, chartVersion)
		require.Equal(t, "fakechart", chartVersion.Name, "name")
		require.Equal(t, "0.1.0", chartVersion.Version, "version")
		require.Equal(t, []string{chartURL}, chartVersion.URLs, "urls")
	})
	t.Run("ResolveChart should resolve repo at alias", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		newSettings := *settings
		newSettings.RepositoryConfig = repoConfPath
		trust := false
		testee := newTestee(t, &newSettings, &trust, g)
		chartVersion, err := testee.ResolveChartVersion(context.Background(), "fakechart", "0.1.0", "@fake-repo")
		require.NoError(t, err)
		require.NotNil(t, chartVersion)
		require.Equal(t, "fakechart", chartVersion.Name, "name")
		require.Equal(t, "0.1.0", chartVersion.Version, "version")
		require.Equal(t, []string{chartURL}, chartVersion.URLs, "urls")
	})
	t.Run("ResolveChart should fail when chart does not exist", func(t *testing.T) {
		g.index = fakeIndexFile(chartURL)
		_, err := testee.ResolveChartVersion(context.Background(), "fakechart", "1.x.x", repoURL)
		require.Error(t, err)
	})
}

func newTestee(t *testing.T, settings *cli.EnvSettings, trust *bool, g *fakeGetter) Interface {
	fakeGetterConstructor := func(_ ...getter.Option) (getter.Getter, error) { return g, nil }
	r, err := New(*settings, getter.Providers([]getter.Provider{{
		Schemes: []string{"fake"},
		New:     fakeGetterConstructor,
	}}), trust)
	require.NoError(t, err)
	return r
}

func fakeIndexFile(chartURL string) *repo.IndexFile {
	return &repo.IndexFile{
		APIVersion: "v2",
		Entries: map[string]repo.ChartVersions{
			"fakechart": []*repo.ChartVersion{
				{
					Metadata: &chart.Metadata{
						APIVersion: "v2",
						Name:       "fakechart",
						Version:    "0.1.0",
					},
					URLs: []string{chartURL},
				},
			},
		},
		PublicKeys: []string{},
	}
}

type fakeGetter struct {
	indexURL string
	chartURL string
	index    *repo.IndexFile
}

func (g *fakeGetter) Get(location string, _ ...getter.Option) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	if location == g.indexURL {
		b, err := yaml.Marshal(g.index)
		if err != nil {
			panic(err)
		}
		_, _ = buf.Write(b)
		return &buf, nil
	} else if location == g.chartURL {
		_, _ = buf.WriteString("fake chart tgz")
		return &buf, nil
	}
	return nil, fmt.Errorf("unexpected location %q", location)
}
