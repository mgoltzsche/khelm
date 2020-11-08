package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mgoltzsche/helm-kustomize-plugin/pkg/helm"
)

const (
	envConfig            = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envRoot              = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
	envAllowUnknownRepos = "HELMR_ALLOW_UNKOWN_REPOSITORIES"
)

func main() {
	log.SetFlags(0)
	rawCfg := os.Getenv(envConfig)
	if len(os.Args) != 2 && rawCfg == "" {
		fmt.Fprintf(os.Stderr, "helm-kustomize-plugin usage: %s [FILE]\n\nENV VARS:\n  %s\n  [%s]\n\nprovided args: %+v\n", os.Args[0], envRoot, envConfig, os.Args[1:])
		os.Exit(1)
	}
	wd, err := os.Getwd()
	handleError(err)
	var baseDir string
	if rawCfg == "" {
		generatorConfigFile := os.Args[1]
		b, err := ioutil.ReadFile(generatorConfigFile)
		handleError(err)
		rawCfg = string(b)
		baseDir = filepath.Dir(generatorConfigFile)
		if !filepath.IsAbs(baseDir) {
			baseDir = filepath.Join(wd, baseDir)
		}
	}
	cfg, err := helm.ReadGeneratorConfig(strings.NewReader(rawCfg))
	handleError(err)
	if baseDir == "" {
		baseDir = wd
	}
	cfg.BaseDir = baseDir
	cfg.RootDir = os.Getenv(envRoot)
	cfg.AllowUnknownRepositories = true
	if allowUnknownReposStr, ok := os.LookupEnv(envAllowUnknownRepos); ok {
		cfg.AllowUnknownRepositories, err = strconv.ParseBool(allowUnknownReposStr)
		if err != nil {
			handleError(fmt.Errorf("parse env var %s: %w", envAllowUnknownRepos, err))
		}
	}
	if cfg.RootDir == "" {
		cfg.RootDir = baseDir
	}
	if !filepath.IsAbs(cfg.RootDir) {
		handleError(fmt.Errorf("env var %s must specify absolute path", envRoot))
	}
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	err = helm.Render(ctx, &cfg.ChartConfig, os.Stdout)
	if helm.IsUnknownRepository(err) {
		err = fmt.Errorf("%w - set env var %s=true to enable", err, envAllowUnknownRepos)
	}
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, os.Args[0]+": %s\n", err)
		os.Exit(2)
	}
}
