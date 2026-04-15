package main

import (
	"fmt"
	"os"

	"github.com/thebargaintenor/prolix-director/cmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: prolix <command> [args]\nCommands: solve")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "solve":
		if err := cmd.RunSolve(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
