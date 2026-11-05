package main

import (
	"os"

	"github.com/mozilla-platform-ops/relops-launcher/internal/launcher"
)

var version = "dev"

func main() {
	os.Exit(launcher.Main(os.Args[1:], version))
}
