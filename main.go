package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/fn/framework"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	envConfig            = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envRoot              = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
	envAllowUnknownRepos = "HELMR_ALLOW_UNKNOWN_REPOSITORIES"
	envDebug             = "HELMR_DEBUG"
)

func main() {
	log.SetFlags(0)
	debug, _ := strconv.ParseBool(os.Getenv(envDebug))
	helm.Debug = helm.Debug || debug

	// Detect kustomize plugin config
	kustomizeGenCfgYAML, isKustomizePlugin := os.LookupEnv(envConfig)
	allowUnknownRepos := true
	if isKustomizePlugin {
		allowUnknownRepos = false
	}
	var err error
	if allowUnknownReposStr, ok := os.LookupEnv(envAllowUnknownRepos); ok {
		allowUnknownRepos, err = strconv.ParseBool(allowUnknownReposStr)
		if err != nil {
			handleError(errors.Wrapf(err, "parse env var %s", envAllowUnknownRepos))
		}
	}

	// Run as kustomize plugin (if called as such)
	if isKustomizePlugin {
		err = runAsKustomizePlugin(kustomizeGenCfgYAML, allowUnknownRepos)
		handleError(err)
		return
	}

	// Run (kpt function) CLI
	kptFnCfg := kptFunctionConfigMap{}
	resourceList := &framework.ResourceList{FunctionConfig: &kptFnCfg}
	cmd := framework.Command(resourceList, func() (err error) {
		rendered, err := render(&kptFnCfg.Data, allowUnknownRepos)
		if err != nil {
			return err
		}
		resourceList.Items = rendered
		return nil
	})
	if err := cmd.Execute(); err != nil {
		os.Stderr.WriteString("\n")
		logStackTrace(err)
		os.Exit(1)
	}
}

func runAsKustomizePlugin(cfgYAML string, allowUnknownRepos bool) error {
	cfg, err := helm.ReadGeneratorConfig(strings.NewReader(cfgYAML))
	if err != nil {
		return err
	}
	resources, err := render(&cfg.ChartConfig, allowUnknownRepos)
	if err != nil {
		return err
	}
	enc := yaml.NewEncoder(os.Stdout)
	for _, r := range resources {
		if err = enc.Encode(r.Document()); err != nil {
			return err
		}
	}
	return enc.Close()
}

type kptFunctionConfigMap struct {
	//Data map[string]string
	Data helm.ChartConfig
}

func render(cfg *helm.ChartConfig, allowUnknownRepos bool) ([]*yaml.RNode, error) {
	cfg.AllowUnknownRepositories = allowUnknownRepos
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	rendered, err := helm.Render(ctx, cfg)
	if helm.IsUnknownRepository(err) {
		log.Printf("HINT: access to unknown repositories can be enabled using env var %s=true", envAllowUnknownRepos)
	}
	return rendered, err
}

func handleError(err error) {
	if err != nil {
		logStackTrace(err)
		log.Fatalf("%s: %s", os.Args[0], err)
	}
}

func logStackTrace(err error) {
	if helm.Debug {
		if e := traceableRootCause(err); e != nil {
			if s := fmt.Sprintf("%+v", e); s != err.Error() {
				log.Println(s)
			}
		}
	}
}

// traceableRootCause unwraps the deepest error that provides a stack trace
func traceableRootCause(err error) error {
	// Unwrap Go 1.13 error or github.com/pkg/errors error
	cause := errors.Unwrap(err)
	if cause != nil && cause != err {
		rootCause := traceableRootCause(cause)
		if hasMoreInfo(rootCause, cause) {
			return rootCause
		}
		if hasMoreInfo(cause, err) {
			return cause
		}
	}
	return err
}

func hasMoreInfo(err, than error) bool {
	s := than.Error()
	for _, l := range strings.Split(fmt.Sprintf("%+v", err), "\n") {
		if !strings.Contains(s, l) {
			return true
		}
	}
	return false
}
