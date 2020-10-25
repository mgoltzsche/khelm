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
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/repo"
)

var currDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}()

func TestRender(t *testing.T) {
	expectedJenkinsContained := "- host: jenkins.example.org\n"
	for _, c := range []struct {
		name              string
		file              string
		expectedNamespace string
		expectedContained string
	}{
		{"jenkins", "../../example/jenkins/jenkins-chart.yaml", "jenkins", expectedJenkinsContained},
		{"values-external", "chartwithextvalues.yaml", "jenkins", expectedJenkinsContained},
		{"rook-ceph", "../../example/rook-ceph/operator/rook-ceph-chart.yaml", "rook-ceph-system", "rook-ceph-v0.9.3"},
		{"cert-manager", "../../example/cert-manager/cert-manager-chart.yaml", "cert-manager", "chart: cainjector-v0.9.1"},
		{"apiversions-condition", "../../example/apiversions-condition/chartref.yaml", "apiversions-condition-env", "  config: fancy-config"},
		{"local-chart-with-remote-dependency", "../../example/localref/chartref.yaml", "myns", "http://efk-elasticsearch-client:9200"},
		{"local-chart-with-local-dependency", "../../example/localrefref/chartref.yaml", "myotherns", "http://efk-elasticsearch-client:9200"},
		{"values-inheritance", "../../example/values-inheritance/chartref.yaml", "values-inheritance-env", "<inherited:inherited value> <fileoverwrite:overwritten by file> <valueoverwrite:overwritten by generator config>"},
		{"unsupported-field", "../../example/unsupported-field/chartref.yaml", "rook-ceph-system", "rook-ceph"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, cached := range []string{"", "cached "} {
				var rendered bytes.Buffer
				absFile := filepath.Join(currDir, c.file)
				rootDir := filepath.Join(currDir, "..", "..")
				err := renderFile(t, absFile, rootDir, &rendered)
				require.NoError(t, err, "render %s%s", cached, absFile)
				b := rendered.Bytes()
				l, err := readYaml(b)
				require.NoError(t, err, "rendered %syaml:\n%s", cached, b)
				require.True(t, len(l) > 0, "%s: rendered result of %s is empty", cached, c.file)
				require.Contains(t, rendered.String(), c.expectedContained, "%syaml", cached)
				hasExpectedNamespace := false
				for _, o := range l {
					if o["metadata"].(map[string]interface{})["namespace"] == c.expectedNamespace {
						hasExpectedNamespace = true
						break
					}
				}
				require.True(t, hasExpectedNamespace, "%s%s: should have namespace %q", cached, c.file, c.expectedNamespace)
			}
		})
	}
}

func TestRenderRejectFileOutsideProjectDir(t *testing.T) {
	file := filepath.Join(currDir, "chartwithextvalues.yaml")
	err := renderFile(t, file, currDir, &bytes.Buffer{})
	require.Error(t, err, "render %s within %s", file, currDir)
}

func TestRenderError(t *testing.T) {
	for _, file := range []string{
		"../../example/invalid-requirements-lock/chartref.yaml",
	} {
		file = filepath.Join(currDir, file)
		rootDir := filepath.Join(currDir, "..", "..")
		err := renderFile(t, file, rootDir, &bytes.Buffer{})
		require.Error(t, err, "render %s", file)
	}
}

// TODO: enable this when repo indices of preconfigured repos are fetched
func disabledTestRenderRepositoryCredentials(t *testing.T) {
	// Make sure a fake chart exists that the fake server can serve
	rootDir := filepath.Join(currDir, "..", "..")
	err := renderFile(t, filepath.Join(rootDir, "example/localrefref/chartref.yaml"), rootDir, &bytes.Buffer{})
	require.NoError(t, err)
	fakeChartTgz := filepath.Join(currDir, "../../example/localrefref/charts/efk-0.1.1.tgz")

	// Create input chart config and fake private chart server
	var cfg ChartConfig
	cfg.Chart = "private-chart"
	cfg.Version = fmt.Sprintf("0.0.%d", time.Now().Unix())
	cfg.RootDir = currDir
	cfg.BaseDir = currDir
	repoEntry := &repo.Entry{
		Name:     "myprivaterepo",
		Username: "fakeuser",
		Password: "fakepassword",
	}
	srv := httptest.NewServer(&fakePrivateChartServerHandler{repoEntry, &cfg.LoaderConfig, fakeChartTgz})
	defer srv.Close()
	cfg.Repository = srv.URL
	repoEntry.URL = cfg.Repository

	// Generate temp repository configuration pointing to fake private server
	tmpHelmHome, err := ioutil.TempDir("", "helmrndr-home-")
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

	err = render(t, &cfg, &bytes.Buffer{})
	require.NoError(t, err, "render chart with repository credentials")
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
				URLs: []string{f.repo.URL + chartFilePath},
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

func renderFile(t *testing.T, file, rootDir string, writer io.Writer) error {
	f, err := os.Open(file)
	require.NoError(t, err)
	defer f.Close()
	cfg, err := ReadGeneratorConfig(f)
	require.NoError(t, err, "ReadGeneratorConfig(%s)", file)
	cfg.RootDir = rootDir
	cfg.BaseDir = filepath.Dir(file)
	return render(t, &cfg.ChartConfig, writer)
}

func render(t *testing.T, cfg *ChartConfig, writer io.Writer) error {
	log.SetFlags(0)
	return Render(context.Background(), cfg, writer)
}

func readYaml(y []byte) (l []map[string]interface{}, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(y))
	o := map[string]interface{}{}
	for ; err == nil; err = dec.Decode(o) {
		if len(o) > 0 {
			l = append(l, o)
			o = map[string]interface{}{}
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}
