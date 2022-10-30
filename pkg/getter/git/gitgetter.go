package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	helmgetter "helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	helmyaml "sigs.k8s.io/yaml"
)

var (
	Schemes     = []string{"git+https", "git+ssh"}
	gitCheckout = gitCheckoutImpl
)

type HelmPackageFunc func(ctx context.Context, path, repoDir string) (string, error)

type RepositoriesFunc func() (repositories.Interface, error)

func New(settings *cli.EnvSettings, reposFn RepositoriesFunc, packageFn HelmPackageFunc) helmgetter.Constructor {
	return func(o ...helmgetter.Option) (helmgetter.Getter, error) {
		repos, err := reposFn()
		if err != nil {
			return nil, err
		}
		return &gitIndexGetter{
			settings:  settings,
			repos:     repos,
			packageFn: packageFn,
		}, nil
	}
}

type gitIndexGetter struct {
	repos     repositories.Interface
	settings  *cli.EnvSettings
	Getters   helmgetter.Providers
	packageFn HelmPackageFunc
}

func (g *gitIndexGetter) Get(location string, options ...helmgetter.Option) (*bytes.Buffer, error) {
	ctx := context.Background()
	ref, err := parseURL(location)
	if err != nil {
		return nil, err
	}
	if ref.Ref == "" {
		log.Println("WARNING: Specifying a git URL without the ref parameter may return a cached, outdated version")
	}
	isRepoIndex := path.Base(ref.Path) == "index.yaml"
	var b []byte
	if isRepoIndex {
		// Generate repo index from directory
		ref = ref.Dir()
		repoDir, err := download(ctx, ref, g.settings.RepositoryCache, g.repos)
		if err != nil {
			return nil, err
		}
		dir := filepath.Join(repoDir, filepath.FromSlash(ref.Path))
		idx, err := generateRepoIndex(dir, g.settings.RepositoryCache, ref)
		if err != nil {
			return nil, fmt.Errorf("generate git repo index: %w", err)
		}
		b, err = helmyaml.Marshal(idx)
		if err != nil {
			return nil, fmt.Errorf("marshal generated repo index: %w", err)
		}
	} else {
		// Build and package chart
		ref.Path = strings.TrimSuffix(ref.Path, ".tgz")
		chartPath := filepath.FromSlash(ref.Path)
		repoDir, err := download(ctx, ref, g.settings.RepositoryCache, g.repos)
		if err != nil {
			return nil, err
		}
		if _, e := os.Stat(filepath.Join(filepath.Join(repoDir, chartPath), "Chart.yaml")); e == nil {
			tmpDir, err := os.MkdirTemp("", "khelm-git-")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(tmpDir)
			tmpRepoDir := filepath.Join(tmpDir, "repo")
			tmpChartDir := filepath.Join(tmpRepoDir, chartPath)
			err = copyDir(repoDir, tmpRepoDir)
			if err != nil {
				return nil, fmt.Errorf("make temp git repo copy: %w", err)
			}
			tgzFile, err := g.packageFn(ctx, tmpChartDir, tmpRepoDir)
			if err != nil {
				return nil, fmt.Errorf("package %s: %w", location, err)
			}
			b, err = os.ReadFile(tgzFile)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("unsupported git location: %s", location)
		}
	}
	var buf bytes.Buffer
	_, err = buf.Write(b)
	return &buf, err
}

func generateRepoIndex(dir, cacheDir string, u *gitURL) (*repo.IndexFile, error) {
	idx := repo.NewIndexFile()
	idx.APIVersion = "v2"
	idx.Entries = map[string]repo.ChartVersions{}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() {
			chartDir := filepath.Join(dir, file.Name())
			chartYamlFile := filepath.Join(chartDir, "Chart.yaml")
			b, err := os.ReadFile(chartYamlFile)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			chrt := chart.Metadata{}
			err = helmyaml.Unmarshal(b, &chrt)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", chartYamlFile, err)
			}
			idx.Entries[file.Name()] = repo.ChartVersions{
				{
					Metadata: &chrt,
					URLs:     []string{"git+" + u.JoinPath(file.Name()+".tgz").String()},
				},
			}
		}
	}
	return idx, nil
}

func download(ctx context.Context, ref *gitURL, cacheDir string, repos repositories.Interface) (string, error) {
	repoRef := *ref
	repoRef.Path = ""
	cacheKey := fmt.Sprintf("sha256-%x", sha256.Sum256([]byte(repoRef.String())))
	cacheDir = filepath.Join(cacheDir, "git")
	destDir := filepath.Join(cacheDir, cacheKey)

	if _, e := os.Stat(destDir); os.IsNotExist(e) {
		auth, _, err := repos.Get("git+" + ref.String())
		if err != nil {
			return "", err
		}
		err = os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return "", err
		}
		tmpDir, err := os.MkdirTemp(cacheDir, ".tmp-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		tmpRepoDir := tmpDir
		err = gitCheckout(ctx, ref.Repo, ref.Ref, auth, tmpRepoDir)
		if err != nil {
			return "", err
		}
		if err = os.Rename(tmpRepoDir, destDir); err != nil && !os.IsExist(err) {
			return "", err
		}
	}
	return destDir, nil
}

func copyDir(srcDir, dstDir string) error {
	files, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	err = os.Mkdir(dstDir, 0750)
	if err != nil {
		return err
	}
	for _, file := range files {
		name := file.Name()
		if name == ".git" {
			continue
		}
		srcFile := filepath.Join(srcDir, name)
		dstFile := filepath.Join(dstDir, name)
		if file.IsDir() {
			err = copyDir(srcFile, dstFile)
			if err != nil {
				return err
			}
		} else if file.Type() == os.ModeSymlink {
			linkDest, err := os.Readlink(srcFile)
			if err != nil {
				return err
			}
			err = os.Symlink(srcFile, linkDest)
			if err != nil {
				return err
			}
		} else if file.Type() != os.ModeCharDevice && file.Type() != os.ModeDevice && file.Type() != os.ModeIrregular {
			err = copyFile(srcFile, dstFile)
			if err != nil {
				return fmt.Errorf("copy %s to %s", srcFile, dstFile)
			}
		} else {
			log.Printf("WARNING: ignoring file with unsupported type (%d) found at %s", file.Type(), srcFile)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	fin, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fin.Close()
	fout, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fout.Close()
	_, err = io.Copy(fout, fin)
	if err != nil {
		return err
	}
	return nil
}
