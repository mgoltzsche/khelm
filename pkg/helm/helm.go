package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/ghodss/yaml"
	any "github.com/golang/protobuf/ptypes/any"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/downloader"
	helmgetter "k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/repo"
	"k8s.io/helm/pkg/resolver"
)

const (
	generatorAPIVersion = "helm.kustomize.mgoltzsche.github.com/v1"
	generatorKind       = "ChartRenderer"
)

var (
	whitespaceRegex    = regexp.MustCompile(`^\s*$`)
	defaultKubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)
)

// Helm type
type Helm struct {
	getters  helmgetter.Providers
	settings environment.EnvSettings
	out      io.Writer
}

// GeneratorConfig define the kustomize plugin's input file content
type GeneratorConfig struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   K8sMetadata `yaml:"metadata"`
	ChartConfig
}

// K8sMetadata define the name to be kubernetes object schema conform
type K8sMetadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

// ChartConfig define chart lookup and render config
type ChartConfig struct {
	HelmHome string `yaml: "helmHome"`
	LoadChartConfig
	RenderConfig
}

// LoadChartConfig define the configuration to load a chart
type LoadChartConfig struct {
	Repository string `yaml:"repository"`
	Chart      string `yaml:"chart"`
	Version    string `yaml:"version"`
	LockFile   string `yaml:"lockFile,omitempty"`
	Verify     bool   `yaml:"verify,omitempty"`
	Keyring    string `yaml:"keyring,omitempty"`
}

// RenderConfig defines the configuration to render a chart
type RenderConfig struct {
	Name        string                 `yaml:"name,omitempty"`
	Namespace   string                 `yaml:"namespace,omitempty"`
	ValueFiles  []string               `yaml:"valueFiles,omitempty"`
	Values      map[string]interface{} `yaml:"values,omitempty"`
	APIVersions []string               `yaml:"apiVersions,omitempty"`
	Exclude     []K8sObjectID          `yaml:"exclude,omitempty"`
	BaseDir     string                 `yaml:"-"`
	RootDir     string                 `yaml:"-"`
}

// ReadGeneratorConfig read the generator configuration
func ReadGeneratorConfig(reader io.Reader) (cfg *GeneratorConfig, err error) {
	b, err := ioutil.ReadAll(reader)
	if err == nil {
		cfg = &GeneratorConfig{}
		err = yaml.Unmarshal(b, cfg)
	}
	if err == nil {
		if cfg.Namespace == "" {
			cfg.Namespace = cfg.Metadata.Namespace
		} else if cfg.Metadata.Namespace != "" && err == nil {
			err = errors.New("both metadata.namespace and namespace defined")
		}
		if cfg.Version == "" && cfg.Repository != "" {
			err = errors.New("no chart version but repository specified")
		}
		if cfg.Chart == "" {
			err = errors.New("chart not specified")
		}
		if cfg.Kind != generatorKind {
			err = errors.Errorf("expected kind %s but was %s", generatorKind, cfg.Kind)
		}
		if cfg.APIVersion != generatorAPIVersion {
			err = errors.Errorf("expected apiVersion %s but was %s", generatorAPIVersion, cfg.APIVersion)
		}
	}
	return cfg, errors.Wrap(err, "read chart renderer config")
}

// Render manifest from helm chart configuration (shorthand)
func Render(ctx context.Context, cfg *GeneratorConfig, writer io.Writer) (err error) {
	h := NewHelm("", os.Stderr)

	if cfg.Repository == "" {
		if cfg.Chart != "." && !strings.HasPrefix(cfg.Chart, "./") {
			return fmt.Errorf("chart name must start with ./ if no repository specified")
		}
		cfg.Chart, err = securePath(cfg.Chart, cfg.BaseDir, cfg.RootDir)
		if err != nil {
			return fmt.Errorf("no repository specified and invalid local chart path provided: %w", err)
		}
	}

	chrt, err := h.LoadChart(&cfg.LoadChartConfig)
	if err != nil {
		return err
	}
	renderCfg := &cfg.RenderConfig
	if renderCfg.Name == "" {
		renderCfg.Name = chrt.Metadata.Name
	}
	return h.RenderChart(chrt, renderCfg, writer)
}

// NewHelm constructs helm
func NewHelm(home string, out io.Writer) *Helm {
	settings := environment.EnvSettings{
		Home: helmpath.Home(environment.DefaultHelmHome),
	}
	if home != "" {
		settings.Home = helmpath.Home(home)
	}
	return &Helm{
		helmgetter.All(settings), // getters(settings),
		settings,
		out,
	}
}

// Initialize initialize the helm home directory.
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/installer/init.go
func (h *Helm) Initialize() (err error) {
	if _, e := os.Stat(h.settings.Home.String()); e == nil {
		return
	}

	log.Printf("Initializing helm home at %s\n", h.settings.Home)

	// Create directories
	home := h.settings.Home
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
			return
		}
	}

	// Create repo file
	repoFile := home.RepositoryFile()
	f := repo.NewRepoFile()
	return f.WriteFile(repoFile, 0644)
}

// Load a given chart, ensuring all dependencies are present in /charts
//
// If given a non-empty lockfilePath, the corresponding file will be injected
// in the returned chart as requirements.lock.
func loadChartWithLockfile(chartPath, lockfilePath string) (c *chart.Chart, err error) {
	// Check chart requirements to make sure all dependencies are present in /charts
	if c, err = chartutil.Load(chartPath); err != nil {
		return
	}

	if lockfilePath != "" {
		var data []byte
		data, err = ioutil.ReadFile(lockfilePath)
		if err != nil {
			return
		}

		found := false
		for _, f := range c.Files {
			if f.TypeUrl == "requirements.lock" {
				found = true
				f.Value = data
			}
		}

		if !found {
			c.Files = append(c.Files, &any.Any{
				TypeUrl: "requirements.lock",
				Value:   data,
			})
		}
	}

	return
}

type repoAuth struct {
	Username string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	CertFile string `yaml:"certFile,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
}

func (h *Helm) findRepoAuth(repoURL string) (auth repoAuth, err error) {
	repos, err := repo.LoadRepositoriesFile(h.settings.Home.RepositoryFile())
	if err != nil {
		return auth, err
	}
	for _, r := range repos.Repositories {
		if r.URL == repoURL {
			auth.Username = r.Username
			auth.Password = r.Password
			auth.CAFile = r.CAFile
			auth.CertFile = r.CertFile
			auth.KeyFile = r.KeyFile
			return auth, nil
		}
	}
	return auth, nil
}

// LoadChart download a chart or load it from cache
func (h *Helm) LoadChart(ref *LoadChartConfig) (c *chart.Chart, err error) {
	if err = h.Initialize(); err != nil {
		return
	}

	auth, err := h.findRepoAuth(ref.Repository)
	if err != nil {
		return
	}

	chartPath, err := h.LocateChartPath(
		ref.Repository,
		auth.Username,
		auth.Password,
		ref.Chart,
		ref.Version,
		ref.Verify,
		ref.Keyring,
		auth.CertFile,
		auth.KeyFile,
		auth.CAFile,
	)
	if err != nil {
		return
	}

	log.Printf("Using chart path %v", chartPath)

	// Check chart requirements to make sure all dependencies are present in /charts
	if c, err = loadChartWithLockfile(chartPath, ref.LockFile); err != nil {
		return
	}

	lock, err := chartutil.LoadRequirementsLock(c)
	if err == chartutil.ErrLockfileNotFound {
		err = nil
	} else if err != nil {
		return
	}

	req, err := chartutil.LoadRequirements(c)
	if err == chartutil.ErrRequirementsNotFound {
		err = nil
	} else if err != nil {
		return
	}

	if req != nil {
		if lock != nil {
			if sum, err := resolver.HashReq(req); err != nil || sum != lock.Digest {
				return nil, fmt.Errorf("requirements.lock is out of sync with requirements.yaml")
			}
		}

		if err = renderutil.CheckDependencies(c, req); err != nil {
			man := &downloader.Manager{
				Out:        h.out,
				ChartPath:  chartPath,
				HelmHome:   h.settings.Home,
				Keyring:    ref.Keyring,
				SkipUpdate: true,
				Getters:    h.getters,
			}

			if err = man.Update(); err != nil {
				return
			}

			c, err = loadChartWithLockfile(chartPath, ref.LockFile)
			if err != nil {
				return
			}
		}
	}

	return
}

// RenderChart manifest
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/template.go
func (h *Helm) RenderChart(chrt *chart.Chart, c *RenderConfig, writer io.Writer) (err error) {
	namespace := c.Namespace
	if namespace == "" {
		namespace = "default" // avoids kustomize panic due to missing namespace
	}
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Name:      c.Name,
			Namespace: namespace,
		},
		KubeVersion: defaultKubeVersion,
	}
	if len(c.APIVersions) > 0 {
		renderOpts.APIVersions = append(c.APIVersions, "v1")
	}
	log.Printf("Rendering chart with name %q, namespace: %q\n", c.Name, namespace)

	rawVals, err := h.Vals(chrt, c.ValueFiles, c.Values, c.RootDir, c.BaseDir, "", "", "")
	if err != nil {
		return errors.Wrap(err, "load values")
	}
	config := &chart.Config{Raw: string(rawVals), Values: map[string]*chart.Value{}}

	renderedTemplates, err := renderutil.Render(chrt, config, renderOpts)
	if err != nil {
		return errors.Wrap(err, "render chart")
	}

	listManifests := manifest.SplitManifests(renderedTemplates)
	exclusions := Matchers(c.Exclude)

	for _, m := range sortByKind(listManifests) {
		b := filepath.Base(m.Name)
		if b == "NOTES.txt" || strings.HasPrefix(b, "_") || whitespaceRegex.MatchString(m.Content) {
			continue
		}
		if err = transform(&m, namespace, exclusions); err != nil {
			return errors.WithMessage(err, filepath.Base(m.Name))
		}
		fmt.Fprintf(writer, "---\n# Source: %s\n", m.Name)
		fmt.Fprintln(writer, m.Content)
	}

	for _, exclusion := range exclusions {
		if !exclusion.Matched {
			return errors.Errorf("exclusion selector did not match: %#v", exclusion.K8sObjectID)
		}
	}

	return
}

func transform(m *manifest.Manifest, namespace string, excludes []*K8sObjectMatcher) error {
	obj, err := ParseObjects(bytes.NewReader([]byte(m.Content)))
	if err != nil {
		return errors.Errorf("%s: %q", err, m.Content)
	}
	obj.ApplyDefaultNamespace(namespace)
	obj.Remove(excludes)
	m.Content = obj.Yaml()
	return nil
}

func securePaths(paths []string, baseDir, rootDir string) (secured []string, err error) {
	secured = make([]string, len(paths))
	for i, path := range paths {
		if secured[i], err = securePath(path, baseDir, rootDir); err != nil {
			return
		}
	}
	return
}

func securePath(path, baseDir, rootDir string) (secured string, err error) {
	if rootDir == "" {
		return "", errors.New("no root dir provided")
	}
	if filepath.IsAbs(path) {
		if path, err = filepath.Rel(rootDir, path); err != nil {
			return
		}
	} else {
		if baseDir, err = filepath.Rel(rootDir, baseDir); err != nil {
			return
		}
		path = filepath.Join(baseDir, path)
	}
	return securejoin.SecureJoin(rootDir, path)
}

// LocateChartPath looks for a chart directory in known places, and returns either the full path or an error.
//
// This does not ensure that the chart is well-formed; only that the requested filename exists.
//
// Order of resolution:
// - current working directory
// - if path is absolute or begins with '.', error out here
// - chart repos in $HELM_HOME
// - URL
//
// If 'verify' is true, this will attempt to also verify the chart.
func (h *Helm) LocateChartPath(repoURL, username, password, name, version string, verify bool, keyring,
	certFile, keyFile, caFile string) (string, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if fi, err := os.Stat(name); err == nil {
		abs, err := filepath.Abs(name)
		if err != nil {
			return abs, err
		}
		if verify {
			if fi.IsDir() {
				return "", errors.New("cannot verify a directory")
			}
			if _, err := downloader.VerifyChart(abs, keyring); err != nil {
				return "", err
			}
		}
		return abs, nil
	}

	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, fmt.Errorf("path %q not found", name)
	}

	crepo := filepath.Join(h.settings.Home.Repository(), name)

	if _, err := os.Stat(crepo); err == nil {
		return filepath.Abs(crepo)
	}

	dl := downloader.ChartDownloader{
		HelmHome: h.settings.Home,
		Out:      h.out,
		Keyring:  keyring,
		Getters:  h.getters,
		Username: username,
		Password: password,
	}

	if verify {
		dl.Verify = downloader.VerifyAlways
	}

	if repoURL != "" {
		chartURL, err := repo.FindChartInAuthRepoURL(repoURL, username, password, name, version,
			certFile, keyFile, caFile, h.getters)
		if err != nil {
			return "", err
		}
		name = chartURL
	}

	if _, err := os.Stat(h.settings.Home.Archive()); os.IsNotExist(err) {
		os.MkdirAll(h.settings.Home.Archive(), 0744)
	}

	log.Printf("Downloading chart %q version %q with user: %q, passwd: %v, keyring: %q\n", name, version, dl.Username, dl.Password != "", dl.Keyring)
	filename, _, err := dl.DownloadTo(name, version, h.settings.Home.Archive())

	if err != nil {
		return filename, err
	}

	return filepath.Abs(filename)
}

// Vals merges values from files specified via -f/--values and
// directly via --set or --set-string or --set-file, marshaling them to YAML
func (h *Helm) Vals(chrt *chart.Chart, valueFiles []string, values map[string]interface{}, rootDir, baseDir, certFile, keyFile, caFile string) (b []byte, err error) {
	base := map[string]interface{}{}
	for _, filePath := range valueFiles {
		currentMap := map[string]interface{}{}
		if b, err = h.readValuesFile(chrt, filePath, rootDir, baseDir, certFile, keyFile, caFile); err != nil {
			return
		}
		if err = yaml.Unmarshal(b, &currentMap); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %s", filePath, err)
		}
		mergeValues(base, currentMap)
	}
	base = mergeValues(base, values)
	return yaml.Marshal(base)
}

//readFile load a file from the local directory or a remote file with a url.
func (h *Helm) readValuesFile(chrt *chart.Chart, filePath, rootDir, baseDir, CertFile, KeyFile, CAFile string) (b []byte, err error) {
	u, err := url.Parse(filePath)
	if u.Scheme == "" || strings.ToLower(u.Scheme) == "file" {
		// Load from local file, fallback to chart file
		var kustomizeFilePath string
		if kustomizeFilePath, err = securePath(filePath, baseDir, rootDir); err != nil {
			return
		}
		if b, err = ioutil.ReadFile(kustomizeFilePath); os.IsNotExist(err) {
			// Fallback to chart file
			filePath = filepath.Clean(filePath)
			for _, f := range chrt.Files {
				if f.GetTypeUrl() == filePath {
					return f.GetValue(), nil
				}
			}
		}
		return
	} else if err != nil {
		return
	}

	// Load file from supported helm getter URL
	getterConstructor, err := h.getters.ByScheme(u.Scheme)
	if err != nil {
		return
	}
	getter, err := getterConstructor(filePath, CertFile, KeyFile, CAFile)
	if err != nil {
		return
	}
	data, err := getter.Get(filePath)
	return data.Bytes(), err
}

func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}
