package main

import (
	"fmt"
	"os"

	"github.com/lanechi/gonex/gx/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gx:", err)
		os.Exit(1)
	}
}
