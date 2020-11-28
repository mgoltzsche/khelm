package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/khelm/pkg/helm"
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

func render(h *helm.Helm, req *helm.ChartConfig) ([]*yaml.RNode, error) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	rendered, err := h.Render(ctx, req)
	if helm.IsUntrustedRepository(err) {
		log.Printf("HINT: access to untrusted repositories can be enabled using env var %s=true or option --%s", envTrustAnyRepo, flagTrustAnyRepo)
	}
	return rendered, err
}
