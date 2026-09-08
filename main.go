package main

import (
	"os"

	"github.com/simon-em/kman/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args))
}
