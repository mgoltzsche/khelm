package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	helmyaml "github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"k8s.io/helm/pkg/getter"
	cli "k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/repo"
)

var rootDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "..", "..")
}()

func TestRender(t *testing.T) {
	expectedJenkinsContained := "- host: \"jenkins.example.org\"\n"
	for _, c := range []struct {
		name              string
		file              string
		expectedNamespace string
		expectedContained string
	}{
		{"jenkins", "example/jenkins/generator.yaml", "jenkins", expectedJenkinsContained},
		{"values-external", "pkg/helm/generatorwithextvalues.yaml", "jenkins", expectedJenkinsContained},
		{"rook-ceph-version-range", "example/rook-ceph/operator/generator.yaml", "rook-ceph-system", "rook-ceph-v0.9.3"},
		{"cert-manager", "example/cert-manager/generator.yaml", "cert-manager", "chart: cainjector-v0.9.1"},
		{"apiversions-condition", "example/apiversions-condition/generator.yaml", "apiversions-condition-env", "  config: fancy-config"},
		{"no-namespace", "example/no-namespace/generator.yaml", "", "  key: a"},
		{"kubeVersion", "example/no-namespace/generator.yaml", "", "  k8sVersion: v1.17.0"},
		{"release-name", "example/no-namespace/generator.yaml", "", "  name: no-namespace-myconfigb"},
		{"exclude", "example/exclude/generator.yaml", "myns", "  key: b"},
		{"local-chart-with-local-dependency-and-transitive-remote", "example/localrefref/generator.yaml", "myotherns", "http://efk-elasticsearch-client:9200"},
		{"local-chart-with-remote-dependency", "example/localref/generator.yaml", "myns", "http://efk-elasticsearch-client:9200"},
		{"values-inheritance", "example/values-inheritance/generator.yaml", "values-inheritance-env", " inherited: inherited value\n  fileoverwrite: overwritten by file\n  valueoverwrite: overwritten by generator config"},
		{"unsupported-field", "example/unsupported-field/generator.yaml", "myns", "key: a\n"},
		{"cluster-scoped", "example/cluster-scoped/generator.yaml", "", "myrolebinding"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, cached := range []string{"", "cached "} {
				var rendered bytes.Buffer
				absFile := filepath.Join(rootDir, c.file)
				err := renderFile(t, absFile, true, rootDir, &rendered)
				require.NoError(t, err, "render %s%s", cached, absFile)
				b := rendered.Bytes()
				l, err := readYaml(b)
				require.NoError(t, err, "rendered %syaml:\n%s", cached, b)
				require.True(t, len(l) > 0, "%s: rendered result of %s is empty", cached, c.file)
				require.Contains(t, rendered.String(), c.expectedContained, "%syaml", cached)
				foundNs := ""
				for _, o := range l {
					var ok bool
					foundNs, ok = o["metadata"].(map[string]interface{})["namespace"].(string)
					if ok {
						require.NotEmpty(t, foundNs, "%s%s: output resource has empty namespace set explicitly", cached, c.file)
						break
					}
				}
				require.Equal(t, c.expectedNamespace, foundNs, "%s%s: namespace in output resource", cached, c.file)
			}
		})
	}
}

func TestRenderUntrustedRepositoryError(t *testing.T) {
	dir, err := ioutil.TempDir("", "khelm-test-untrusted-repo-")
	require.NoError(t, err)
	defer os.RemoveAll(dir)
	os.Setenv("HELM_HOME", dir)
	defer os.Unsetenv("HELM_HOME")

	file := filepath.Join(rootDir, "example/rook-ceph/operator/generator.yaml")
	err = renderFile(t, file, false, rootDir, &bytes.Buffer{})
	require.Error(t, err, file)
}

func TestRenderInvalidRequirementsLockError(t *testing.T) {
	file := filepath.Join(rootDir, "example/invalid-requirements-lock/generator.yaml")
	err := renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.Error(t, err, "render %s", file)
}

func TestRenderUnexpectedClusterScopedResourcesError(t *testing.T) {
	file := filepath.Join(rootDir, "example/cluster-scoped-forbidden/generator.yaml")
	err := renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.Error(t, err, "render %s", file)
}

func TestRenderExclude(t *testing.T) {
	file := filepath.Join(rootDir, "example/exclude/generator.yaml")
	buf := bytes.Buffer{}
	err := renderFile(t, file, true, rootDir, &buf)
	require.NoError(t, err, "render %s", file)
	require.Contains(t, buf.String(), "myconfigb")
	require.NotContains(t, buf.String(), "myconfiga")
}

func TestRenderExclusionNoMatchError(t *testing.T) {
	file := filepath.Join(rootDir, "example/exclude-nomatch/generator.yaml")
	buf := bytes.Buffer{}
	err := renderFile(t, file, true, rootDir, &buf)
	require.Error(t, err, "render %s", file)
}

func TestRenderRebuildsLocalDependencies(t *testing.T) {
	tplDir := filepath.Join(rootDir, "example/localref/elk/templates")
	tplFile := filepath.Join(tplDir, "changed.yaml")
	configFile := filepath.Join(rootDir, "example/localrefref/generator.yaml")
	os.RemoveAll(tplDir)

	// Render once to ensure the dependency has been built already
	err := renderFile(t, configFile, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "1st render")

	// Change the dependency
	err = os.Mkdir(tplDir, 0755)
	require.NoError(t, err)
	defer os.RemoveAll(tplDir)
	data := []byte("apiVersion: fancyapi/v1\nkind: FancyKind\nmetadata:\n  name: sth\nchangedField: changed-value")
	err = ioutil.WriteFile(tplFile, data, 0644)
	require.NoError(t, err)

	// Render again and verify that the dependency is rebuilt
	var rendered bytes.Buffer
	err = renderFile(t, configFile, true, rootDir, &rendered)
	require.NoError(t, err, "render after dependency has changed")
	require.Contains(t, rendered.String(), "changedField: changed-value", "local dependency changes should be reflected within the rendered output")
}

func TestRenderUpdateRepositoryIndexIfChartNotFound(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "khelm-test-")
	defer os.RemoveAll(tmpDir)
	settings := cli.EnvSettings{Home: helmpath.Home(tmpDir)}
	repoURL := "https://charts.rook.io/stable"
	trust := true
	repos, err := reposForURLs(map[string]struct{}{repoURL: {}}, &trust, &settings, getter.All(settings))
	require.NoError(t, err, "use repo")
	entry, err := repos.Get(repoURL)
	require.NoError(t, err, "repos.EntryByURL()")
	err = repos.Close()
	require.NoError(t, err, "repos.Close()")
	err = os.MkdirAll(settings.Home.Cache(), 0755)
	require.NoError(t, err)
	idxFile := indexFile(entry, settings.Home.Cache())
	idx := repo.NewIndexFile() // write empty index file to cause not found error
	err = idx.WriteFile(idxFile, 0644)
	require.NoError(t, err, "write empty index file")

	file := filepath.Join(rootDir, "example/rook-ceph/operator/generator.yaml")
	err = renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "render %s with outdated index", file)
}

func TestRenderUpdateRepositoryIndexIfDependencyNotFound(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "khelm-test-")
	defer os.RemoveAll(tmpDir)
	settings := cli.EnvSettings{Home: helmpath.Home(tmpDir)}
	repoURL := "https://kubernetes-charts.storage.googleapis.com"
	trust := true
	repos, err := reposForURLs(map[string]struct{}{repoURL: {}}, &trust, &settings, getter.All(settings))
	require.NoError(t, err, "use repo")
	entry, err := repos.Get(repoURL)
	require.NoError(t, err, "repos.Get()")
	err = repos.Close()
	require.NoError(t, err, "repos.Close()")
	err = os.MkdirAll(settings.Home.Cache(), 0755)
	require.NoError(t, err)
	idxFile := indexFile(entry, settings.Home.Cache())
	idx := repo.NewIndexFile() // write empty index file to cause not found error
	err = idx.WriteFile(idxFile, 0644)
	require.NoError(t, err, "write empty index file")
	err = os.RemoveAll(filepath.Join(rootDir, "example/localref/elk/charts"))
	require.NoError(t, err, "remove charts")

	file := filepath.Join(rootDir, "example/localref/generator.yaml")
	err = renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "render %s with outdated index", file)
}

func TestRenderRepositoryCredentials(t *testing.T) {
	// Make sure a fake chart exists that the fake server can serve
	err := renderFile(t, filepath.Join(rootDir, "example/localrefref/generator.yaml"), true, rootDir, &bytes.Buffer{})
	require.NoError(t, err)
	fakeChartTgz := filepath.Join(rootDir, "example/localrefref/charts/efk-0.1.1.tgz")

	// Create input chart config and fake private chart server
	var cfg ChartConfig
	cfg.Chart = "private-chart"
	cfg.Name = "myrelease"
	cfg.Version = fmt.Sprintf("0.0.%d", time.Now().Unix())
	cfg.BaseDir = rootDir
	repoEntry := &repo.Entry{
		Name:     "myprivaterepo",
		Username: "fakeuser",
		Password: "fakepassword",
	}
	srv := httptest.NewServer(&fakePrivateChartServerHandler{repoEntry, &cfg.LoaderConfig, fakeChartTgz})
	defer srv.Close()
	repoEntry.URL = srv.URL

	// Generate temp repository configuration pointing to fake private server
	tmpHelmHome, err := ioutil.TempDir("", "khelm-test-home-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpHelmHome)
	origHelmHome := os.Getenv("HELM_HOME")
	err = os.Setenv("HELM_HOME", tmpHelmHome)
	require.NoError(t, err)
	defer os.Setenv("HELM_HOME", origHelmHome)
	repos := repo.NewRepoFile()
	repos.Add(repoEntry)
	b, err := yaml.Marshal(repos)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(tmpHelmHome, "repository"), 0755)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tmpHelmHome, "repository", "repositories.yaml"), b, 0644)
	require.NoError(t, err)

	for _, c := range []struct {
		name string
		repo string
	}{
		{"url", repoEntry.URL},
		{"alias", "@" + repoEntry.Name},
		{"aliasScheme", "alias:" + repoEntry.Name},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg.Repository = c.repo
			err = render(t, cfg, false, &bytes.Buffer{})
			require.NoError(t, err, "render chart with repository credentials")
		})
	}
}

type fakePrivateChartServerHandler struct {
	repo         *repo.Entry
	config       *LoaderConfig
	fakeChartTgz string
}

func (f *fakePrivateChartServerHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	usr, pwd, ok := req.BasicAuth()
	if !ok || usr != f.repo.Username || pwd != f.repo.Password {
		writer.WriteHeader(401)
		return
	}
	chartFilePath := fmt.Sprintf("/%s-%s.tgz", f.config.Chart, f.config.Version)
	switch req.RequestURI {
	case "/index.yaml":
		idx := repo.NewIndexFile()
		idx.APIVersion = "v1"
		idx.Entries = map[string]repo.ChartVersions{
			f.config.Chart: {{
				Metadata: &chart.Metadata{
					ApiVersion: "v1",
					AppVersion: f.config.Version,
					Version:    f.config.Version,
					Name:       f.config.Chart,
				},
				Digest: "0000000000000000",
				URLs:   []string{f.repo.URL + chartFilePath},
			}},
		}
		b, err := helmyaml.Marshal(idx)
		if err != nil {
			log.Println("ERROR: fake server:", err)
			writer.WriteHeader(500)
			return
		}
		writer.WriteHeader(200)
		writer.Write(b)
		return
	case chartFilePath:
		writer.WriteHeader(200)
		f, err := os.Open(f.fakeChartTgz)
		if err == nil {
			defer f.Close()
			_, err = io.Copy(writer, f)
		}
		if err != nil {
			log.Println("ERROR: fake server:", err)
		}
		return
	}
	log.Println("ERROR: fake server received unexpected request:", req.RequestURI)
	writer.WriteHeader(404)
}

func renderFile(t *testing.T, file string, trustAnyRepo bool, rootDir string, writer io.Writer) error {
	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()
	cfg, err := ReadGeneratorConfig(f)
	require.NoError(t, err, "ReadGeneratorConfig(%s)", file)
	cfg.BaseDir = filepath.Dir(file)
	return render(t, cfg.ChartConfig, trustAnyRepo, writer)
}

func render(t *testing.T, req ChartConfig, trustAnyRepo bool, writer io.Writer) error {
	log.SetFlags(0)
	h := NewHelm()
	h.TrustAnyRepository = &trustAnyRepo
	resources, err := h.Render(context.Background(), &req)
	if err != nil {
		return err
	}
	enc := yaml.NewEncoder(writer)
	enc.SetIndent(2)
	for _, r := range resources {
		enc.Encode(r.Document())
	}
	return enc.Close()
}

func readYaml(y []byte) (l []map[string]interface{}, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	o := map[string]interface{}{}
	i := 0
	for ; err == nil; err = dec.Decode(o) {
		if len(o) > 0 {
			l = append(l, o)
			o = map[string]interface{}{}
		}
		i++
	}
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		err = fmt.Errorf("invalid yaml output at resource %d: %w", i, err)
	}
	return
}
