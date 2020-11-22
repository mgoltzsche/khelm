package cmd

import (
	"io"
	"strings"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"github.com/mgoltzsche/helmr/pkg/internal/output"
)

func runAsKustomizePlugin(h *helm.Helm, generatorYAML string, writer io.Writer) error {
	req, err := helm.ReadGeneratorConfig(strings.NewReader(generatorYAML))
	if err != nil {
		return err
	}
	resources, err := render(h, &req.ChartConfig)
	if err != nil {
		return err
	}
	return output.Marshal(resources, writer)
}
