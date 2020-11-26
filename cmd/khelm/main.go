package main

import (
	"log"
	"os"
)

func main() {
	if err := Execute(os.Stdin, os.Stdout); err != nil {
		log.Fatalf("khelm: %s", err)
	}
}
