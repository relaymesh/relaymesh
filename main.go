package main

import (
	"fmt"
	"os"

	"github.com/relaymesh/relaymesh/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
