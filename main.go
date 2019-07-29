package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mgoltzsche/helm-kustomize-plugin/pkg/helm"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "helm-kustomize-plugin usage: %s FILE\nprovided args: %+v\n", os.Args[0], os.Args[1:])
		os.Exit(1)
	}
	log.SetFlags(0)
	f, err := os.Open(os.Args[1])
	handleError(err)
	defer f.Close()
	fmt.Fprintf(os.Stderr, "##ARGS: %s %s\n", os.Args[0], os.Args[1])
	cfg, err := helm.ReadGeneratorConfig(f)
	handleError(err)
	err = helm.Render(cfg, os.Stdout)
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, os.Args[0]+": %s\n", err)
		os.Exit(2)
	}
}
