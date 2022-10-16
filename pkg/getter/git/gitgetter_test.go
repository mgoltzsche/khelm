package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

func TestGitGetter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "khelm-git-getter-test-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	gitCheckout = func(_ context.Context, repoURL, ref string, auth *repo.Entry, destDir string) error {
		err := os.MkdirAll(filepath.Join(destDir, "mypath", "fakechartdir"), 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(destDir, "mypath", "fakechartdir", "Chart.yaml"), []byte(`
apiVersion: v2
name: fake-chart
version: 0.1.0`), 0600)
		require.NoError(t, err)
		return nil
	}
	packageCalls := 0
	settings := cli.New()
	settings.RepositoryCache = filepath.Join(tmpDir, "cache")
	fakePackageFn := func(ctx context.Context, path, repoDir string) (string, error) {
		packageCalls++
		file := filepath.Join(tmpDir, "fake-chart.tgz")
		err := os.WriteFile(file, []byte("fake-chart-tgz-contents "+path), 0600)
		require.NoError(t, err)
		return file, nil
	}
	reposFn := func() (repositories.Interface, error) {
		trust := true
		return repositories.New(*settings, getter.All(settings), &trust)
	}
	testee := New(settings, reposFn, fakePackageFn)
	getter, err := testee()
	require.NoError(t, err)

	b, err := getter.Get("git+https://git.example.org/org/repo@mypath/index.yaml?ref=v0.6.2")
	require.NoError(t, err)
	idx := repo.NewIndexFile()
	err = yaml.Unmarshal(b.Bytes(), idx)
	require.NoErrorf(t, err, "unmarshal Get() result: %s", b.String())
	expect := repo.NewIndexFile()
	expect.APIVersion = "v2"
	fakeChartURL := "git+https://git.example.org/org/repo@mypath/fakechartdir.tgz?ref=v0.6.2"
	expect.Entries = map[string]repo.ChartVersions{
		"fakechartdir": []*repo.ChartVersion{
			{
				Metadata: &chart.Metadata{
					APIVersion: "v2",
					Name:       "fake-chart",
					Version:    "0.1.0",
				},
				URLs: []string{fakeChartURL},
			},
		},
	}
	expect.Generated = idx.Generated
	require.Equal(t, expect, idx, "should generate repository index from directory")

	b, err = getter.Get(fakeChartURL)
	require.NoError(t, err)
	require.Contains(t, b.String(), "fake-chart-tgz-contents /tmp/khelm-git-", "should return packaged chart")
	require.Truef(t, strings.HasSuffix(b.String(), "/repo/mypath/fakechartdir"), "should end with path within repo but was: %s", b.String())
}
