package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mgoltzsche/helm-kustomize-plugin/pkg/helm"
)

const (
	envConfig = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envRoot   = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
)

func main() {
	log.SetFlags(0)
	rawCfg := os.Getenv(envConfig)
	if len(os.Args) != 2 && rawCfg == "" {
		fmt.Fprintf(os.Stderr, "helm-kustomize-plugin usage: %s [FILE]\n\nENV VARS:\n  %s\n  [%s]\n\nprovided args: %+v\n", os.Args[0], envRoot, envConfig, os.Args[1:])
		os.Exit(1)
	}
	if rawCfg == "" {
		b, err := ioutil.ReadFile(os.Args[1])
		handleError(err)
		rawCfg = string(b)
	}
	cfg, err := helm.ReadGeneratorConfig(strings.NewReader(rawCfg))
	handleError(err)
	cfg.RootDir = os.Getenv(envRoot)
	if cfg.RootDir == "" {
		handleError(fmt.Errorf("no %s env var provided", envRoot))
	}
	cfg.BaseDir, err = os.Getwd()
	handleError(err)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	err = helm.Render(ctx, cfg, os.Stdout)
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, os.Args[0]+": %s\n", err)
		os.Exit(2)
	}
}
