package helm

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
)

const requirementsLockFile = "requirements.lock"

type chartDescriptor struct {
	Name         string                      `yaml:"name"`
	Version      string                      `yaml:"version"`
	Dependencies []chartDescriptorDependency `yaml:"dependencies"`
}

type chartDescriptorDependency struct {
	Name       string `yaml:"name"`
	Version    string `yaml:"version"`
	Repository string `yaml:"repository"`
}

type tempChart struct {
	tmpDir   string
	lockFile string
	//chartFile string
}

func (c *tempChart) Close() (err error) {
	defer func() {
		// Remove temp chart
		if e := os.RemoveAll(c.tmpDir); e != nil && err == nil {
			err = e
		}
		if err != nil {
			err = fmt.Errorf("clean up temp chart: %w", err)
		}
	}()
	// Copy temporary lockFile to persistent location
	tmpLockFile := filepath.Join(c.tmpDir, requirementsLockFile)
	lockFileExists := true
	if c.lockFile != "" {
		_, e := os.Stat(c.lockFile)
		lockFileExists = e == nil
		if !lockFileExists {
			err = copyFileAtomic(tmpLockFile, c.lockFile)
			if err != nil {
				return fmt.Errorf("copy lockFile: %w", err)
			}
		}
	}
	// Copy temporary chartFile to persistent location
	/*if c.chartFile != "" {
		_, e = os.Stat(c.chartFile)
		if chartFileExists := e == nil; !chartFileExists || !lockFileExists {
			tgzName, e := chartFileNameFromLockFile(tmpLockFile)
			if e != nil {
				return e
			}
			//panic(c.chartFile)
			err = copyFileAtomic(filepath.Join(c.tmpDir, "charts", tgzName), c.chartFile)
			if err != nil {
				return fmt.Errorf("copy chartFile: %w", err)
			}
		}
	}*/
	return
}

func newTempChart(cfg *GeneratorConfig) (chrt *tempChart, err error) {
	b, err := yaml.Marshal(&chartDescriptor{
		Name:    "temp-chart",
		Version: "0.0.0",
		Dependencies: []chartDescriptorDependency{
			{
				Name:       cfg.Chart,
				Version:    cfg.Version,
				Repository: cfg.Repository,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	tmpDir, err := ioutil.TempDir("", "chart-")
	if err != nil {
		return nil, fmt.Errorf("create temp chart: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpDir)
		}
	}()
	err = ioutil.WriteFile(filepath.Join(tmpDir, "Chart.yaml"), b, 0640)
	if err != nil {
		return nil, fmt.Errorf("create temp chart: %w", err)
	}
	if cfg.LockFile != "" {
		err = copyFileIfSrcExists(cfg.LockFile, filepath.Join(tmpDir, requirementsLockFile))
		if err != nil {
			return nil, fmt.Errorf("copy %s to temp requirements.lock: %w", cfg.LockFile, err)
		}

		/*if cfg.ChartFile != "" {
			if _, err = os.Stat(cfg.ChartFile); err == nil {
				tgzName, e := chartFileNameFromLockFile(cfg.LockFile)
				if e != nil {
					return nil, e
				}
				if err = os.MkdirAll(filepath.Join(tmpDir, "charts"), 0755); err != nil {
					return nil, err
				}
				tmpTgzFile := filepath.Join(tmpDir, "charts", tgzName)
				if err = os.Link(cfg.ChartFile, tmpTgzFile); err != nil {
					if err = copyFileIfSrcExists(cfg.ChartFile, tmpTgzFile); err != nil {
						return nil, err
					}
				}
			}
		}*/
	}
	chrt = &tempChart{tmpDir, cfg.LockFile}
	cfg.Chart = tmpDir
	cfg.Repository = ""
	cfg.Version = ""
	cfg.ValueFiles = nil
	cfg.Values = nil
	return chrt, nil
}

func chartFileNameFromLockFile(lockFile string) (string, error) {
	lock := &chart.Lock{}
	b, err := ioutil.ReadFile(lockFile)
	if err != nil {
		return "", err
	}
	err = yaml.Unmarshal(b, lock)
	if err != nil {
		return "", err
	}
	return chartFileNameFromLock(lock), nil
}

func singleDependency(lock *chart.Lock) *chart.Dependency {
	if len(lock.Dependencies) != 1 {
		panic(fmt.Errorf("expected 1 chart dependency but %d found", len(lock.Dependencies)))
	}
	d := lock.Dependencies[0]
	return d
}

func chartFileNameFromLock(lock *chart.Lock) string {
	d := singleDependency(lock)
	return fmt.Sprintf("%s-%s.tgz", d.Name, d.Version)
}

func copyFileIfSrcExists(srcFile, destFile string) (err error) {
	src, err := os.Open(srcFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		return err
	}
	defer func() {
		if e := dst.Close(); e != nil && err == nil {
			err = e
		}
	}()
	_, err = io.Copy(dst, src)
	return err
}

func copyFileAtomic(srcFile, destFile string) (err error) {
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()
	return writeFileAtomic(src, destFile)
}
