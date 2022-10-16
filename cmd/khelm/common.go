package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mgoltzsche/khelm/v2/pkg/config"
	"github.com/mgoltzsche/khelm/v2/pkg/helm"
	"github.com/mgoltzsche/khelm/v2/pkg/repositories"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func render(h *helm.Helm, req *config.ChartConfig) ([]*yaml.RNode, error) {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigs
		log.Printf("Received %s signal", s)
		cancel()
	}()

	rendered, err := h.Render(ctx, req)
	if repositories.IsUntrustedRepository(err) {
		log.Printf("HINT: access to untrusted repositories can be enabled using env var %s=true or option --%s", envTrustAnyRepo, flagTrustAnyRepo)
	}
	return rendered, err
}
