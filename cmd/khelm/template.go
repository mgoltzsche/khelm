package main

import (
	"fmt"
	"io"

	"github.com/mgoltzsche/khelm/v1/internal/output"
	"github.com/mgoltzsche/khelm/v1/pkg/helm"
	"github.com/spf13/cobra"
	"k8s.io/helm/pkg/strvals"
)

func templateCommand(h *helm.Helm, writer io.Writer) *cobra.Command {
	req := helm.NewChartConfig()
	req.Name = "release-name"
	outOpts := output.Options{Writer: writer}
	trustAnyRepo := false
	cmd := &cobra.Command{
		Use: "template",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				_ = cmd.Help()
				return fmt.Errorf("accepts single CHART argument but received %d arguments", len(args))
			}
			return nil
		},
		SuggestFor: []string{"render", "build"},
		Short:      "Renders a chart",
		Example:    usageExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed(flagTrustAnyRepo) {
				h.TrustAnyRepository = &trustAnyRepo
			}
			out, err := output.New(outOpts)
			if err != nil {
				return err
			}
			req.Chart = args[0]
			resources, err := render(h, req)
			if err != nil {
				return err
			}
			return out.Write(resources)
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		_ = cmd.Help()
		return err
	})
	f := cmd.Flags()
	f.StringVar(&req.Repository, "repo", "", "Chart repository url where to locate the requested chart")
	f.StringVar(&req.Repository, "repository", "", "Chart repository url where to locate the requested chart")
	f.Lookup("repository").Hidden = true
	f.StringVar(&req.Version, "version", "", "Specify the exact chart version to use. If this is not specified, the latest version is used")
	f.BoolVar(&trustAnyRepo, flagTrustAnyRepo, trustAnyRepo,
		fmt.Sprintf("Allow to use repositories that are not registered within repositories.yaml (default is true when repositories.yaml does not exist; %s)", envTrustAnyRepo))
	f.BoolVar(&req.NamespacedOnly, "namespaced-only", false, "Fail on known cluster-scoped resources and those of unknown kinds")
	f.BoolVar(&req.Verify, "verify", false, "Verify the package before using it")
	f.StringVar(&req.Keyring, "keyring", req.Keyring, "Keyring used to verify the chart")
	f.StringVar(&req.Name, "name", req.Name, "Release name")
	f.StringVar(&req.Namespace, "namespace", req.Namespace, "Set the installation namespace used by helm templates")
	f.StringVar(&req.ForceNamespace, "force-namespace", req.ForceNamespace, "Set namespace on all namespaced resources (and those of unknown kinds)")
	f.Var((*valuesFlag)(&req.Values), "set", "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
	f.StringSliceVarP(&req.ValueFiles, "values", "f", nil, "Specify values in a YAML file or a URL (can specify multiple)")
	f.StringSliceVar(&req.APIVersions, "api-versions", nil, "Kubernetes api versions used for Capabilities.APIVersions")
	f.StringVar(&req.KubeVersion, "kube-version", req.KubeVersion, "Kubernetes version used as Capabilities.KubeVersion.Major/Minor")
	f.StringVarP(&outOpts.FileOrDir, "output", "o", "-", "Write rendered output to given file or directory (as kustomization)")
	f.BoolVar(&outOpts.Replace, "output-replace", false, "Delete and recreate the whole output directory or file")
	return cmd
}

type valuesFlag map[string]interface{}

func (f *valuesFlag) Set(s string) error {
	base := *(*map[string]interface{})(f)
	return strvals.ParseInto(s, base)
}

func (f *valuesFlag) Type() string {
	return "strings"
}

func (f *valuesFlag) String() string {
	return ""
}
