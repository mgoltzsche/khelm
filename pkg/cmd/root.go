package cmd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/mgoltzsche/helmr/pkg/helm"
)

const (
	envConfig         = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envRoot           = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
	envAcceptAnyRepo  = "HELMR_ACCEPT_ANY_REPO"
	envDebug          = "HELMR_DEBUG"
	envHelmDebug      = "HELM_DEBUG"
	flagAcceptAnyRepo = "accept-any-repo"
)

var usageExample = fmt.Sprintf("  %s template ./chart\n  %s template stable/jenkins\n  %s template jenkins --version=2.5.3 --repo=https://kubernetes-charts.storage.googleapis.com", os.Args[0], os.Args[0], os.Args[0])

// Execute runs the helmr CLI
func Execute(writer io.Writer) error {
	log.SetFlags(0)
	debug, _ := strconv.ParseBool(os.Getenv(envDebug))
	helmDebug, _ := strconv.ParseBool(os.Getenv(envHelmDebug))
	helmCfg := helm.NewConfig()
	helmCfg.Debug = debug || helmDebug
	if acceptAnyRepo, ok := os.LookupEnv(envAcceptAnyRepo); ok {
		allow, _ := strconv.ParseBool(acceptAnyRepo)
		helmCfg.AcceptAnyRepository = &allow
	}

	// Run as kustomize plugin (if kustomize-specific env var provided)
	if kustomizeGenCfgYAML, isKustomizePlugin := os.LookupEnv(envConfig); isKustomizePlugin {
		err := runAsKustomizePlugin(helmCfg, kustomizeGenCfgYAML, writer)
		logStackTrace(err, helmCfg.Debug)
		return err
	}

	// Add kpt function command
	errBuf := bytes.Buffer{}
	cmd := kptFnCommand(&helmCfg)
	cmd.SetOut(writer)
	cmd.SetErr(&errBuf)
	cmd.PersistentFlags().BoolVar(&debug, "debug", debug, fmt.Sprintf("enable debug log (%s)", envDebug))

	// Add template command (for non-kpt usage)
	templateCmd := templateCommand(helmCfg, writer)
	templateCmd.SetOut(writer)
	templateCmd.SetErr(&errBuf)
	cmd.AddCommand(templateCmd)

	// Run command
	helmCfg.Debug = true
	if err := cmd.Execute(); err != nil {
		logStackTrace(err, helmCfg.Debug)
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			err = fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
