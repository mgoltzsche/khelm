package main

import (
	"io"
	"strings"

	"github.com/mgoltzsche/khelm/internal/output"
	"github.com/mgoltzsche/khelm/pkg/helm"
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
