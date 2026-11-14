// Command swapsafe verifies that a Gateway API setup reproduces ingress-nginx behaviour.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/tuan-nguyenduc/swapsafe/internal/cli"

	// Registers every ingress-nginx rule through its init functions.
	_ "github.com/tuan-nguyenduc/swapsafe/rules/ingressnginx"
)

func main() {
	err := cli.NewRootCommand().Execute()
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, "swapsafe:", err)

	// A run that finished and found differences exits 2, so CI can tell it
	// apart from swapsafe failing to run at all.
	var coded cli.Coder
	if errors.As(err, &coded) {
		os.Exit(coded.Code())
	}
	os.Exit(cli.ExitError)
}
