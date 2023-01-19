package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mgoltzsche/khelm/v2/pkg/getter/git"
	"github.com/mgoltzsche/khelm/v2/pkg/helm"
	"github.com/spf13/cobra"
	helmgetter "helm.sh/helm/v3/pkg/getter"
)

const (
	envKustomizePluginConfig     = "KUSTOMIZE_PLUGIN_CONFIG_STRING"
	envKustomizePluginConfigRoot = "KUSTOMIZE_PLUGIN_CONFIG_ROOT"
	envEnableGitGetter           = "KHELM_ENABLE_GIT_GETTER"
	envTrustAnyRepo              = "KHELM_TRUST_ANY_REPO"
	envDebug                     = "KHELM_DEBUG"
	envHelmDebug                 = "HELM_DEBUG"
	flagTrustAnyRepo             = "trust-any-repo"
	usageExample                 = "  khelm template ./chart\n  khelm template stable/jenkins\n  khelm template jenkins --version=2.5.3 --repo=https://kubernetes-charts.storage.googleapis.com"
)

var (
	khelmVersion = "dev-build"
	helmVersion  = "unknown-version"
)

// Execute runs the khelm CLI
func Execute(reader io.Reader, writer io.Writer) error {
	log.SetFlags(0)
	debug, _ := strconv.ParseBool(os.Getenv(envDebug))
	helmDebug, _ := strconv.ParseBool(os.Getenv(envHelmDebug))
	h := helm.NewHelm()
	debug = debug || helmDebug
	enableGitGetter := false
	h.Settings.Debug = debug
	if trustAnyRepo, ok := os.LookupEnv(envTrustAnyRepo); ok {
		trust, _ := strconv.ParseBool(trustAnyRepo)
		h.TrustAnyRepository = &trust
	}
	if gitSupportStr, ok := os.LookupEnv(envEnableGitGetter); ok {
		enableGitGetter, _ = strconv.ParseBool(gitSupportStr)
	}

	// Run as kustomize plugin (if kustomize-specific env var provided)
	if kustomizeGenCfgYAML, isKustomizePlugin := os.LookupEnv(envKustomizePluginConfig); isKustomizePlugin {
		logVersion()
		err := runAsKustomizePlugin(h, kustomizeGenCfgYAML, writer)
		logStackTrace(err, debug)
		return err
	}

	preRun := func(_ *cobra.Command, _ []string) {
		logVersion()
		if enableGitGetter {
			addGitGetterSupport(h)
		}
	}
	rootCmd := &cobra.Command{
		PersistentPreRun: preRun,
	}
	rootCmd.PersistentFlags().BoolVar(&enableGitGetter, "enable-git-getter", enableGitGetter, fmt.Sprintf("enable git+https helm repository URL scheme support (%s)", envEnableGitGetter))
	errBuf := bytes.Buffer{}

	if filepath.Base(os.Args[0]) == "khelmfn" {
		// Add kpt function command
		rootCmd = krmFnCommand(h)
		rootCmd.SetIn(reader)
		rootCmd.SetOut(writer)
		rootCmd.SetErr(&errBuf)
		rootCmd.PersistentFlags().BoolVar(&debug, "debug", debug, fmt.Sprintf("enable debug log (%s)", envDebug))
		rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
			fmt.Printf("# Reading kpt function input from stdin (use `%s template` to run without kpt)\n", os.Args[0])
		}
	}

	rootCmd.AddCommand(versionCmd)
	rootCmd.Example = usageExample
	rootCmd.Use = "khelm"
	rootCmd.Short = fmt.Sprintf("khelm %s chart renderer", khelmVersion)
	rootCmd.Long = `khelm is a helm chart templating CLI, kpt function and kustomize plugin.

In addition to helm's templating capabilities khelm allows to:
 * build local charts automatically when templating
 * use any repository without registering it in repositories.yaml
 * enforce namespace-scoped resources within the template output
 * set a namespace on all resources
 * convert a helm chart's output into a kustomization`

	// Add template command (for non-kpt usage)
	templateCmd := templateCommand(h, writer)
	templateCmd.SetOut(writer)
	templateCmd.SetErr(&errBuf)
	templateCmd.PreRun = preRun
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

func logVersion() {
	log.Println("Running khelm", versionInfo())
}

func versionInfo() string {
	return fmt.Sprintf("%s (helm %s)", khelmVersion, helmVersion)
}

func addGitGetterSupport(h *helm.Helm) {
	h.Getters = append(h.Getters, helmgetter.Provider{
		Schemes: git.Schemes,
		New: git.New(&h.Settings, h.Repositories, func(ctx context.Context, chartDir, repoDir string) (string, error) {
			return h.Package(ctx, chartDir, repoDir, chartDir)
		}),
	})
}
