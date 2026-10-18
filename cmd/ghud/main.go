package main

import (
	"os"

	"github.com/oronbz/ghud/internal/cli"
)

func main() {
	os.Exit(cli.Run(cli.System{
		Args:   os.Args[1:],
		Getenv: os.Getenv,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}
