package cmd

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"path/filepath"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/spf13/cobra"
)

const (
	envKustomizePluginConfig     = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envKustomizePluginConfigRoot = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
	envTrustAnyRepo              = "HELMR_TRUST_ANY_REPO"
	envDebug                     = "HELMR_DEBUG"
	envHelmDebug                 = "HELM_DEBUG"
	flagTrustAnyRepo             = "trust-any-repo"
)

var usageExample = fmt.Sprintf("  %s template ./chart\n  %s template stable/jenkins\n  %s template jenkins --version=2.5.3 --repo=https://kubernetes-charts.storage.googleapis.com", os.Args[0], os.Args[0], os.Args[0])

// Execute runs the helmr CLI
func Execute(reader io.Reader, writer io.Writer) error {
	log.SetFlags(0)
	debug, _ := strconv.ParseBool(os.Getenv(envDebug))
	helmDebug, _ := strconv.ParseBool(os.Getenv(envHelmDebug))
	h := helm.NewHelm()
	debug = debug || helmDebug
	h.Settings.Debug = debug
	if trustAnyRepo, ok := os.LookupEnv(envTrustAnyRepo); ok {
		trust, _ := strconv.ParseBool(trustAnyRepo)
		h.TrustAnyRepository = &trust
	}

	// Run as kustomize plugin (if kustomize-specific env var provided)
	if kustomizeGenCfgYAML, isKustomizePlugin := os.LookupEnv(envKustomizePluginConfig); isKustomizePlugin {
		err := runAsKustomizePlugin(h, kustomizeGenCfgYAML, writer)
		logStackTrace(err, debug)
		return err
	}

	rootCmd := &cobra.Command{}
	errBuf := bytes.Buffer{}

	if filepath.Base(os.Args[0]) == "helmrfn" {
		// Add kpt function command
		rootCmd = kptFnCommand(h)
		rootCmd.SetIn(reader)
		rootCmd.SetOut(writer)
		rootCmd.SetErr(&errBuf)
		rootCmd.PersistentFlags().BoolVar(&debug, "debug", debug, fmt.Sprintf("enable debug log (%s)", envDebug))
		rootCmd.PreRun = func(_ *cobra.Command, _ []string) {
			fmt.Printf("# Reading kpt function input from stdin (use `%s template` to run without kpt)\n", os.Args[0])
		}
	}

	rootCmd.Example = usageExample
	rootCmd.Use = os.Args[0]
	rootCmd.Short = "helmr chart renderer"
	rootCmd.Long = `helmr is a helm chart templating CLI, kpt function and kustomize plugin.

In opposite to the original helm CLI helmr allows to
 * build local charts automatically when templating
 * use any repository without registering it in repositories.yaml
 * enforce namespace-scoped resources within the template output
 * set a namespace on all resources
 * convert a helm chart's output into a kustomization`

	// Add template command (for non-kpt usage)
	templateCmd := templateCommand(h, writer)
	templateCmd.SetOut(writer)
	templateCmd.SetErr(&errBuf)
	rootCmd.AddCommand(templateCmd)

	// Run command
	if err := rootCmd.Execute(); err != nil {
		logStackTrace(err, debug)
		msg := strings.TrimSpace(errBuf.String())
		if msg != "" {
			err = fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}
