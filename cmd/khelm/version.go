package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version info",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println(versionInfo())
	},
}
