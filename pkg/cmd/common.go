package cmd

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/helmr/pkg/helm"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func marshal(resources []*yaml.RNode, writer io.Writer) error {
	enc := yaml.NewEncoder(writer)
	for _, r := range resources {
		if err := enc.Encode(r.Document()); err != nil {
			return err
		}
	}
	return enc.Close()
}

func render(cfg helm.Config, req helm.ChartConfig) ([]*yaml.RNode, error) {
	h, err := helm.NewHelm(cfg)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	rendered, err := h.Render(ctx, req)
	if helm.IsUnknownRepository(err) {
		log.Printf("HINT: access to unknown repositories can be enabled using env var %s=true or option --%s", envAcceptAnyRepo, flagAcceptAnyRepo)
	}
	return rendered, err
}
