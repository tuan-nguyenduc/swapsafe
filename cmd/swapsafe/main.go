// Command swapsafe verifies that a Gateway API setup reproduces ingress-nginx behaviour.
package main

import (
	"fmt"
	"os"

	"github.com/tuan-nguyenduc/swapsafe/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "swapsafe:", err)
		os.Exit(1)
	}
}
