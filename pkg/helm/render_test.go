package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	helmyaml "sigs.k8s.io/yaml"
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
		name                  string
		file                  string
		expectedNamespaces    []string
		expectedContained     string
		expectedResourceNames []string
	}{
		{"jenkins", "example/jenkins/generator.yaml", []string{"jenkins"}, expectedJenkinsContained, nil},
		{"values-external", "pkg/helm/generatorwithextvalues.yaml", []string{"jenkins"}, expectedJenkinsContained, nil},
		{"rook-ceph-version-range", "example/rook-ceph/operator/generator.yaml", []string{}, "rook-ceph-v0.9.3", nil},
		{"cert-manager", "example/cert-manager/generator.yaml", []string{"cert-manager", "kube-system"}, " name: cert-manager-webhook", nil},
		{"apiversions-condition", "example/apiversions-condition/generator.yaml", []string{}, "  config: fancy-config", nil},
		{"expand-list", "example/expand-list/generator.yaml", []string{"ns1", "ns2", "ns3"}, "\n  name: myserviceaccount2\n", nil},
		{"namespace", "example/namespace/generator.yaml", []string{"default-namespace", "cluster-role-binding-ns"}, "  key: b", nil},
		{"force-namespace", "example/force-namespace/generator.yaml", []string{"forced-namespace"}, "  key: b", nil},
		{"kubeVersion", "example/release-name/generator.yaml", []string{}, "  k8sVersion: v1.17.0", nil},
		{"release-name", "example/release-name/generator.yaml", []string{}, "  name: my-release-name-config", nil},
		{"exclude", "example/exclude/generator.yaml", []string{"cluster-role-binding-ns"}, "  key: b", nil},
		{"include", "example/include/generator.yaml", []string{}, "  key: b", nil},
		{"local-chart-with-local-dependency-and-transitive-remote", "example/localrefref/generator.yaml", []string{}, "rook-ceph-v0.9.3", nil},
		{"local-chart-with-remote-dependency", "example/localref/generator.yaml", []string{}, "rook-ceph-v0.9.3", nil},
		{"oci-chart", "example/oci-image/generator.yaml", []string{"kube-system", "kube-node-lease"}, "name: ec2nodeclasses.karpenter.k8s.aws", nil},
		{"oci-dependency", "example/oci-dependency/generator.yaml", []string{"kube-system", "kube-node-lease"}, "name: ec2nodeclasses.karpenter.k8s.aws", nil},
		{"values-inheritance", "example/values-inheritance/generator.yaml", []string{}, " inherited: inherited value\n  fileoverwrite: overwritten by file\n  valueoverwrite: overwritten by generator config", nil},
		{"cluster-scoped", "example/cluster-scoped/generator.yaml", []string{}, "myrolebinding", nil},
		{"chart-hooks", "example/chart-hooks/generator.yaml", []string{"default"}, "  key: myvalue", []string{
			"chart-hooks-myconfig",
			"chart-hooks-post-delete",
			"chart-hooks-post-install",
			"chart-hooks-post-upgrade",
			"chart-hooks-pre-delete",
			"chart-hooks-pre-install",
			"chart-hooks-pre-rollback",
			"chart-hooks-pre-upgrade",
			"chart-hooks-test",
		}},
		{"chart-hooks-disabled", "example/chart-hooks-disabled/generator.yaml", []string{"default"}, "  key: myvalue", []string{"chart-hooks-disabled-myconfig"}},
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
				foundResourceNames := []string{}
				foundNamespaces := map[string]struct{}{}
				for i, o := range l {
					require.NotNilf(t, o["metadata"], "%s object %d metadata", cached, i)
					ns := ""
					meta := o["metadata"].(map[string]interface{})
					foundResourceNames = append(foundResourceNames, meta["name"].(string))
					nsVal, ok := meta["namespace"]
					if ok {
						if ns, ok = nsVal.(string); ok {
							foundNamespaces[ns] = struct{}{}
						}
						require.NotEmpty(t, ns, "%s%s: output resource declares empty namespace field", cached, c.file)
					}
					subjects, ok := o["subjects"].([]interface{})
					if ok && len(subjects) > 0 {
						if subject, ok := subjects[0].(map[string]interface{}); ok {
							if ns, ok = subject["namespace"].(string); ok {
								require.NotEmpty(t, ns, "%s%s: output resource has empty subjects[0].namespace set explicitly", cached, c.file)
								foundNamespaces[ns] = struct{}{}
							}
						}
					}
				}

				foundNs := []string{}
				for k := range foundNamespaces {
					foundNs = append(foundNs, k)
				}
				sort.Strings(c.expectedNamespaces)
				sort.Strings(foundNs)
				require.Equal(t, c.expectedNamespaces, foundNs, "%s%s: namespaces of output resource", cached, c.file)

				if len(c.expectedResourceNames) > 0 {
					require.Equal(t, c.expectedResourceNames, foundResourceNames, "resource names")
				}
			}
		})
	}
}

func TestRenderUntrustedRepositoryError(t *testing.T) {
	dir, err := os.MkdirTemp("", "khelm-test-untrusted-repo-")
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
	tplDir := filepath.Join(rootDir, "example/localref/intermediate-chart/templates")
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
	err = os.WriteFile(tplFile, data, 0600)
	require.NoError(t, err)

	// Render again and verify that the dependency is rebuilt
	var rendered bytes.Buffer
	err = renderFile(t, configFile, true, rootDir, &rendered)
	require.NoError(t, err, "render after dependency has changed")
	require.Contains(t, rendered.String(), "changedField: changed-value", "local dependency changes should be reflected within the rendered output")
}

func TestRenderUpdateRepositoryIndexIfChartNotFound(t *testing.T) {
	settings := cli.New()
	repoURL := "https://charts.rook.io/stable"
	trust := true
	repos, err := reposForURLs(map[string]struct{}{repoURL: {}}, &trust, settings, getter.All(settings))
	require.NoError(t, err, "use repo")
	entry, err := repos.Get(repoURL)
	require.NoError(t, err, "repos.EntryByURL()")
	err = repos.Close()
	require.NoError(t, err, "repos.Close()")
	idxFile := indexFile(entry, settings.RepositoryCache)
	idx := repo.NewIndexFile() // write empty index file to cause not found error
	err = idx.WriteFile(idxFile, 0644)
	require.NoError(t, err, "write empty index file")

	file := filepath.Join(rootDir, "example/rook-ceph/operator/generator.yaml")
	err = renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "render %s with outdated index", file)
}

func TestRenderUpdateRepositoryIndexIfDependencyNotFound(t *testing.T) {
	settings := cli.New()
	repoURL := "https://kubernetes-charts.storage.googleapis.com"
	trust := true
	repos, err := reposForURLs(map[string]struct{}{repoURL: {}}, &trust, settings, getter.All(settings))
	require.NoError(t, err, "use repo")
	entry, err := repos.Get(repoURL)
	require.NoError(t, err, "repos.Get()")
	err = repos.Close()
	require.NoError(t, err, "repos.Close()")
	idxFile := indexFile(entry, settings.RepositoryCache)
	idx := repo.NewIndexFile() // write empty index file to cause not found error
	err = idx.WriteFile(idxFile, 0644)
	require.NoError(t, err, "write empty index file")
	err = os.RemoveAll(filepath.Join(rootDir, "example/localref/rook-ceph/charts"))
	require.NoError(t, err, "remove charts")

	file := filepath.Join(rootDir, "example/localref/generator.yaml")
	err = renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "render %s with outdated index", file)
}

func TestRenderNoDigest(t *testing.T) {
	// Make sure a fake chart exists that the fake server can serve
	err := renderFile(t, filepath.Join(rootDir, "example/localrefref/generator.yaml"), true, rootDir, &bytes.Buffer{})
	require.NoError(t, err)
	fakeChartTgz := filepath.Join(rootDir, "example/localrefref/charts/intermediate-chart-0.1.1.tgz")
	digest := ""

	// Create input chart config and fake private chart server
	cfg := config.NewChartConfig()
	cfg.Chart = "private-chart"
	cfg.Name = "myrelease"
	cfg.Version = fmt.Sprintf("0.0.%d", time.Now().Unix())
	cfg.BaseDir = rootDir
	repoEntry := &repo.Entry{
		Name:     "myprivaterepo",
		Username: "fakeuser",
		Password: "fakepassword",
	}
	srv := httptest.NewServer(&fakePrivateChartServerHandler{repoEntry, &cfg.LoaderConfig, fakeChartTgz, digest})
	defer srv.Close()
	repoEntry.URL = srv.URL

	// Generate temp repository configuration pointing to fake private server
	tmpHelmHome, err := os.MkdirTemp("", "khelm-test-home-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpHelmHome)
	origHelmHome := os.Getenv("HELM_HOME")
	err = os.Setenv("HELM_HOME", tmpHelmHome)
	require.NoError(t, err)
	defer os.Setenv("HELM_HOME", origHelmHome)
	repos := repo.NewFile()
	repos.Add(repoEntry)
	b, err := yaml.Marshal(repos)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(tmpHelmHome, "repository"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpHelmHome, "repository", "repositories.yaml"), b, 0600) // #nosec
	require.NoError(t, err)

	cfg.Repository = repoEntry.URL
	err = render(t, *cfg, false, &bytes.Buffer{})
	require.NoError(t, err, "render chart with no digest")
}

func TestRenderRepositoryCredentials(t *testing.T) {
	// Make sure a fake chart exists that the fake server can serve
	err := renderFile(t, filepath.Join(rootDir, "example/localrefref/generator.yaml"), true, rootDir, &bytes.Buffer{})
	require.NoError(t, err)
	fakeChartTgz := filepath.Join(rootDir, "example/localrefref/charts/intermediate-chart-0.1.1.tgz")
	digest := "0000000000000000"

	// Create input chart config and fake private chart server
	cfg := config.NewChartConfig()
	cfg.Chart = "private-chart"
	cfg.Name = "myrelease"
	cfg.Version = fmt.Sprintf("0.0.%d", time.Now().Unix())
	cfg.BaseDir = rootDir
	repoEntry := &repo.Entry{
		Name:     "myprivaterepo",
		Username: "fakeuser",
		Password: "fakepassword",
	}
	srv := httptest.NewServer(&fakePrivateChartServerHandler{repoEntry, &cfg.LoaderConfig, fakeChartTgz, digest})
	defer srv.Close()
	repoEntry.URL = srv.URL

	// Generate temp repository configuration pointing to fake private server
	tmpHelmHome, err := os.MkdirTemp("", "khelm-test-home-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpHelmHome)
	origHelmHome := os.Getenv("HELM_HOME")
	err = os.Setenv("HELM_HOME", tmpHelmHome)
	require.NoError(t, err)
	defer os.Setenv("HELM_HOME", origHelmHome)
	repos := repo.NewFile()
	repos.Add(repoEntry)
	b, err := yaml.Marshal(repos)
	require.NoError(t, err)
	err = os.Mkdir(filepath.Join(tmpHelmHome, "repository"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpHelmHome, "repository", "repositories.yaml"), b, 0600) // #nosec
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
			err = render(t, *cfg, false, &bytes.Buffer{})
			require.NoError(t, err, "render chart with repository credentials")
		})
	}
}

type fakePrivateChartServerHandler struct {
	repo         *repo.Entry
	config       *config.LoaderConfig
	fakeChartTgz string
	digest       string
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
		idx.APIVersion = "v2"
		idx.Entries = map[string]repo.ChartVersions{
			f.config.Chart: {{
				Metadata: &chart.Metadata{
					AppVersion: f.config.Version,
					Version:    f.config.Version,
					Name:       f.config.Chart,
				},
				Digest: f.digest,
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
		_, _ = writer.Write(b)
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
	cfg, err := config.ReadGeneratorConfig(f)
	require.NoError(t, err, "ReadGeneratorConfig(%s)", file)
	cfg.BaseDir = filepath.Dir(file)
	return render(t, cfg.ChartConfig, trustAnyRepo, writer)
}

func render(t *testing.T, req config.ChartConfig, trustAnyRepo bool, writer io.Writer) error {
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
		err = enc.Encode(r.Document())
		if err != nil {
			return err
		}
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

func TestRenderValueFilesProtocol(t *testing.T) {
	file := filepath.Join(rootDir, "example/helm-secrets/generator.yaml")
	os.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(rootDir, "example/helm-secrets/age.txt"))
	origHelmConfigHome := os.Getenv("HELM_CONFIG_HOME")
	defer os.Setenv("HELM_CONFIG_HOME", origHelmConfigHome)
	err := renderFile(t, file, true, rootDir, &bytes.Buffer{})
	require.NoError(t, err, "render chart with valueFile protocol")
}
