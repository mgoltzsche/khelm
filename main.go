package main

import (
	"log"
	"os"

	"github.com/mgoltzsche/helmr/pkg/cmd"
)

func main() {
	if err := cmd.Execute(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("helmr: %s", err)
	}
}
