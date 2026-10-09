package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/skilled-manager/skills-manager/pkg/core"
)

func printUsage() {
	fmt.Print(`Skills Manager CLI - Manage AI Agent Skills

Usage:
  skills <command> [arguments]

Commands:
  list                              List all skills and their target statuses
  enable <skill> [target]           Enable a skill for a target (or all targets)
  disable <skill> [target]          Disable a skill for a target (or all targets)
  sync                              Synchronize all drifted copies, TOMLs, and links
  import [--scan | --all]           Scan or import existing untracked skills
  new <name> <description>          Create a new skill in the canonical store
  export [output.zip]               Export canonical skills to a ZIP archive
  targets                           List configured tool targets

Options:
  -canonical <path>                 Override canonical skills folder
  -config <path>                    Override configuration folder
  -h, --help                        Show help
`)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	var canonicalPath string
	var configPath string

	// Handle flags before or after command
	fs := flag.NewFlagSet("skills", flag.ContinueOnError)
	fs.StringVar(&canonicalPath, "canonical", "", "Canonical skills directory")
	fs.StringVar(&configPath, "config", "", "Config directory")

	command := os.Args[1]
	args := os.Args[2:]

	if command == "-h" || command == "--help" || command == "help" {
		printUsage()
		return
	}

	var opts []core.Option
	if canonicalPath != "" {
		opts = append(opts, core.WithCanonicalDir(canonicalPath))
	}
	if configPath != "" {
		opts = append(opts, core.WithConfigDir(configPath))
	}

	mgr, err := core.NewManager(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing manager: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "list":
		handleList(mgr)

	case "enable":
		if len(args) < 1 {
			fmt.Println("Usage: skills enable <skill-name> [target-id]")
			os.Exit(1)
		}
		skillName := args[0]
		targetID := ""
		if len(args) > 1 {
			targetID = args[1]
		}
		handleEnable(mgr, skillName, targetID)

	case "disable":
		if len(args) < 1 {
			fmt.Println("Usage: skills disable <skill-name> [target-id]")
			os.Exit(1)
		}
		skillName := args[0]
		targetID := ""
		if len(args) > 1 {
			targetID = args[1]
		}
		handleDisable(mgr, skillName, targetID)

	case "sync":
		handleSync(mgr)

	case "import":
		handleImport(mgr, args)

	case "new":
		if len(args) < 2 {
			fmt.Println("Usage: skills new <skill-name> <description>")
			os.Exit(1)
		}
		handleNew(mgr, args[0], args[1])

	case "export":
		outFile := "agent-skills.zip"
		if len(args) > 0 {
			outFile = args[0]
		}
		handleExport(mgr, outFile)

	case "targets":
		handleTargets(mgr)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleList(mgr *core.Manager) {
	skills, err := core.ListSkills(mgr.CanonicalDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing skills: %v\n", err)
		os.Exit(1)
	}

	if len(skills) == 0 {
		fmt.Printf("No skills found in canonical store (%s).\nUse 'skills new <name> <description>' to create one.\n", mgr.CanonicalDir)
		return
	}

	targets := mgr.GetTargets()
	fmt.Printf("Canonical store: %s (%d skills)\n\n", mgr.CanonicalDir, len(skills))

	for _, skill := range skills {
		fmt.Printf("• %s\n", skill.Name)
		if skill.Description != "" {
			fmt.Printf("  Description: %s\n", skill.Description)
		}

		fmt.Print("  Targets: ")
		var statusStrs []string
		for _, target := range targets {
			status, err := mgr.Status(skill.Name, target.ID)
			if err != nil {
				statusStrs = append(statusStrs, fmt.Sprintf("%s: [error: %v]", target.ID, err))
			} else {
				statusStrs = append(statusStrs, fmt.Sprintf("%s: [%s]", target.ID, status.Code))
			}
		}
		fmt.Println(strings.Join(statusStrs, ", "))
		fmt.Println()
	}
}

func handleEnable(mgr *core.Manager, skillName, targetID string) {
	targets := mgr.GetTargets()
	if targetID != "" {
		res, err := mgr.Enable(skillName, targetID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error enabling %q on %s: %v\n", skillName, targetID, err)
			os.Exit(1)
		}
		fmt.Printf("Enabled %q on %s (%s) -> %s\n", skillName, targetID, res.Mode, res.TargetPath)
		return
	}

	// Enable across all targets
	successCount := 0
	for _, target := range targets {
		res, err := mgr.Enable(skillName, target.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed for %s: %v\n", target.Label, err)
		} else {
			fmt.Printf("✓ %s: %s\n", target.Label, res.Message)
			successCount++
		}
	}
	fmt.Printf("Done: enabled %q on %d/%d targets.\n", skillName, successCount, len(targets))
}

func handleDisable(mgr *core.Manager, skillName, targetID string) {
	targets := mgr.GetTargets()
	if targetID != "" {
		err := mgr.Disable(skillName, targetID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error disabling %q on %s: %v\n", skillName, targetID, err)
			os.Exit(1)
		}
		fmt.Printf("Disabled %q on %s\n", skillName, targetID)
		return
	}

	for _, target := range targets {
		err := mgr.Disable(skillName, target.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed for %s: %v\n", target.Label, err)
		} else {
			fmt.Printf("✓ Disabled on %s\n", target.Label)
		}
	}
}

func handleSync(mgr *core.Manager) {
	fmt.Println("Synchronizing skills across all targets...")
	report, err := mgr.Sync()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync error: %v\n", err)
		os.Exit(1)
	}

	if len(report.Updated) > 0 {
		fmt.Printf("\nUpdated (%d):\n", len(report.Updated))
		for _, u := range report.Updated {
			fmt.Printf("  ✓ %s\n", u)
		}
	}

	if len(report.Skipped) > 0 {
		fmt.Printf("\nSkipped collisions (%d):\n", len(report.Skipped))
		for _, s := range report.Skipped {
			fmt.Printf("  ⚠ %s\n", s)
		}
	}

	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(report.Errors))
		for _, e := range report.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
	}

	if len(report.Updated) == 0 && len(report.Skipped) == 0 && len(report.Errors) == 0 {
		fmt.Println("All targets are up to date.")
	}
}

func handleImport(mgr *core.Manager, args []string) {
	candidates, err := mgr.ScanImportCandidates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning targets for import: %v\n", err)
		os.Exit(1)
	}

	if len(candidates) == 0 {
		fmt.Println("No untracked skills found in target directories.")
		return
	}

	importAll := len(args) > 0 && (args[0] == "--all" || args[0] == "-a")

	fmt.Printf("Discovered %d untracked skill(s):\n\n", len(candidates))
	for i, c := range candidates {
		fmt.Printf("[%d] %s (found in %s: %s)\n", i+1, c.SkillName, c.TargetLabel, c.SourcePath)
		if c.Description != "" {
			fmt.Printf("    Description: %s\n", c.Description)
		}
	}
	fmt.Println()

	if !importAll {
		fmt.Println("Run 'skills import --all' to import all discovered skills into the canonical store.")
		return
	}

	imported, errs := mgr.ImportSelected(candidates)
	for _, name := range imported {
		fmt.Printf("✓ Imported and linked: %s\n", name)
	}
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "✗ %v\n", e)
	}
}

func handleNew(mgr *core.Manager, name, description string) {
	initialBody := fmt.Sprintf("# %s\n\nInstructions for %s...\n", name, name)
	skill, err := core.CreateSkill(mgr.CanonicalDir, name, description, initialBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating skill: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created skill %q at %s\n", skill.Name, skill.Path)
}

func handleExport(mgr *core.Manager, outFile string) {
	f, err := os.Create(outFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating zip file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if err := mgr.ExportZip(nil, f); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting zip: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Exported skills archive to %s\n", outFile)
}

func handleTargets(mgr *core.Manager) {
	targets := mgr.GetTargets()
	fmt.Printf("Configured targets (%s):\n\n", mgr.TargetsPath)
	for _, t := range targets {
		fmt.Printf("• %s (%s)\n", t.Label, t.ID)
		fmt.Printf("  Path:   %s (expanded: %s)\n", t.Path, core.ExpandPath(t.Path))
		fmt.Printf("  Mode:   %s\n", t.Mode)
		fmt.Printf("  Format: %s\n\n", t.Format)
	}
}
