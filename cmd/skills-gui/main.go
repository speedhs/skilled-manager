package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/skilled-manager/skills-manager/internal/ui"
	"github.com/skilled-manager/skills-manager/pkg/core"
)

func main() {
	var canonicalPath string
	var configPath string

	flag.StringVar(&canonicalPath, "canonical", "", "Canonical skills directory (default ~/.agent-skills)")
	flag.StringVar(&configPath, "config", "", "Configuration directory (default ~/.agent-skills-manager)")
	flag.Parse()

	var opts []core.Option
	if canonicalPath != "" {
		opts = append(opts, core.WithCanonicalDir(canonicalPath))
	}
	if configPath != "" {
		opts = append(opts, core.WithConfigDir(configPath))
	}

	mgr, err := core.NewManager(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize manager: %v\n", err)
		os.Exit(1)
	}

	guiApp, err := ui.NewSkillsApp(mgr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize GUI: %v\n", err)
		os.Exit(1)
	}

	guiApp.Run()
}
