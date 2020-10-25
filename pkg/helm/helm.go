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

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/downloader"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/helm/environment"
	"k8s.io/helm/pkg/helm/helmpath"
	"k8s.io/helm/pkg/manifest"
	"k8s.io/helm/pkg/proto/hapi/chart"
	"k8s.io/helm/pkg/renderutil"
	"k8s.io/helm/pkg/repo"
	"k8s.io/helm/pkg/resolver"
)

var (
	whitespaceRegex    = regexp.MustCompile(`^\s*$`)
	defaultKubeVersion = fmt.Sprintf("%s.%s", chartutil.DefaultKubeVersion.Major, chartutil.DefaultKubeVersion.Minor)
)

// Render manifest from helm chart configuration (shorthand)
func Render(ctx context.Context, cfg *ChartConfig, writer io.Writer) (err error) {
	h := newHelm("", os.Stderr)

	if cfg.Repository == "" {
		if cfg.Chart != "." && !(strings.HasPrefix(cfg.Chart, "./") || cfg.Chart != ".." && strings.HasPrefix(cfg.Chart, "../")) {
			return errors.New("chart name must start with ./ if no repository specified")
		}
		cfg.Chart, err = securePath(cfg.Chart, cfg.BaseDir, cfg.RootDir)
		if err != nil {
			return errors.Wrap(err, "no repository specified and invalid local chart path provided")
		}
	}

	chrt, err := h.LoadChart(&cfg.LoaderConfig)
	if err != nil {
		return err
	}
	return h.RenderChart(chrt, cfg, writer)
}

// Helm type
type helm struct {
	getters  getter.Providers
	settings environment.EnvSettings
	out      io.Writer
}

// NewHelm constructs helm
func newHelm(home string, out io.Writer) *helm {
	helmHome := os.Getenv("HELM_HOME")
	if helmHome == "" {
		helmHome = environment.DefaultHelmHome
	}
	settings := environment.EnvSettings{
		Home: helmpath.Home(helmHome),
	}
	if home != "" {
		settings.Home = helmpath.Home(home)
	}
	return &helm{
		getter.All(settings), // getters(settings),
		settings,
		out,
	}
}

// Initialize initialize the helm home directory.
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/installer/init.go
func (h *helm) Initialize() (err error) {
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

type repoAuth struct {
	Username string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	CertFile string `yaml:"certFile,omitempty"`
	KeyFile  string `yaml:"keyFile,omitempty"`
	CAFile   string `yaml:"caFile,omitempty"`
}

// LoadChart download a chart or load it from cache
func (h *helm) LoadChart(ref *LoaderConfig) (c *chart.Chart, err error) {
	if err = h.Initialize(); err != nil {
		return
	}

	chartPath, err := h.LocateChartPath(
		ref.Repository,
		ref.Chart,
		ref.Version,
		ref.Verify,
		ref.Keyring,
	)
	if err != nil {
		return nil, err
	}

	log.Printf("Using chart path %v", chartPath)

	if c, err = chartutil.Load(chartPath); err != nil {
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
				return nil, errors.New("requirements.lock is out of sync with requirements.yaml")
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

			if c, err = chartutil.Load(chartPath); err != nil {
				return
			}
		}
	}

	return
}

// RenderChart manifest
// Derived from https://github.com/helm/helm/blob/v2.14.3/cmd/helm/template.go
func (h *helm) RenderChart(chrt *chart.Chart, c *ChartConfig, writer io.Writer) (err error) {
	namespace := c.Namespace
	if namespace == "" {
		namespace = "default" // avoids kustomize panic due to missing namespace
	}
	renderOpts := renderutil.Options{
		ReleaseOptions: chartutil.ReleaseOptions{
			Name:      c.ReleaseName,
			Namespace: namespace,
		},
		KubeVersion: defaultKubeVersion,
	}
	if len(c.APIVersions) > 0 {
		renderOpts.APIVersions = append(c.APIVersions, "v1")
	}
	log.Printf("Rendering chart with name %q, namespace: %q\n", c.ReleaseName, namespace)

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
func (h *helm) LocateChartPath(repoURL, name, version string, verify bool, keyring string) (string, error) {
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
		return name, errors.Errorf("path %q not found", name)
	}

	crepo := filepath.Join(h.settings.Home.Repository(), name)

	if _, err := os.Stat(crepo); err == nil {
		return filepath.Abs(crepo)
	}

	repos, err := reposForURLs(&h.settings, h.getters, map[string]struct{}{repoURL: {}})
	if err != nil {
		return "", err
	}
	tmpRepos, err := repos.Apply()
	if err != nil {
		return "", err
	}
	defer tmpRepos.Close()
	if err = tmpRepos.UpdateIndex(); err != nil {
		return "", err
	}
	h.settings.Home = tmpRepos.HelmHome()
	/*isRange, err := isVersionRange(cfg.Version)
	if err != nil {
		return err
	}
	if isRange {
		if err = repos.UpdateIndex(); err != nil {
			return err
		}
	}*/

	r, err := repos.EntryByURL(repoURL)
	if err != nil {
		return "", err
	}

	dl := downloader.ChartDownloader{
		HelmHome: h.settings.Home,
		Out:      h.out,
		Keyring:  keyring,
		Getters:  h.getters,
		Username: r.Username,
		Password: r.Password,
	}

	if verify {
		dl.Verify = downloader.VerifyAlways
	}

	if _, err := os.Stat(h.settings.Home.Archive()); os.IsNotExist(err) {
		os.MkdirAll(h.settings.Home.Archive(), 0744)
	}

	log.Printf("Downloading chart %q version %q with user: %q, passwd: %v, keyring: %q\n", name, version, dl.Username, dl.Password != "", dl.Keyring)
	filename, _, err := dl.DownloadTo(fmt.Sprintf("%s/%s", r.Name, name), version, h.settings.Home.Archive())

	if err != nil {
		return filename, errors.Wrapf(err, "download chart %s %s", name, version)
	}

	return filepath.Abs(filename)
}

// Vals merges values from files specified via -f/--values and
// directly via --set or --set-string or --set-file, marshaling them to YAML
func (h *helm) Vals(chrt *chart.Chart, valueFiles []string, values map[string]interface{}, rootDir, baseDir, certFile, keyFile, caFile string) (b []byte, err error) {
	base := map[string]interface{}{}
	for _, filePath := range valueFiles {
		currentMap := map[string]interface{}{}
		if b, err = h.readValuesFile(chrt, filePath, rootDir, baseDir, certFile, keyFile, caFile); err != nil {
			return
		}
		if err = yaml.Unmarshal(b, &currentMap); err != nil {
			return nil, errors.Wrapf(err, "failed to parse %s", filePath)
		}
		mergeValues(base, currentMap)
	}
	base = mergeValues(base, values)
	return yaml.Marshal(base)
}

//readFile load a file from the local directory or a remote file with a url.
func (h *helm) readValuesFile(chrt *chart.Chart, filePath, rootDir, baseDir, CertFile, KeyFile, CAFile string) (b []byte, err error) {
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
