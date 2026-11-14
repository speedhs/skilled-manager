// Package cli is the skills command-line interface.
// os.Exit is called only from cmd/skills. This package returns an exit code.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/speedhs/skilled-manager/internal/version"
	"github.com/speedhs/skilled-manager/pkg/core"
)

const (
	// ExitOK means the command finished without an error.
	ExitOK = 0
	// ExitError means the command failed.
	ExitError = 1
	// ExitUsage means the arguments were wrong.
	ExitUsage = 2
	// ExitConflict means a destination is not one this tool created, or both sides changed.
	ExitConflict = 3
	// ExitPartial means some targets succeeded and some failed.
	ExitPartial = 4
)

// Run executes args, which do not include the program name.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, rest, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	if opts.help || len(rest) == 0 {
		fmt.Fprint(stdout, usageText)
		if len(rest) == 0 && !opts.help {
			return ExitUsage
		}
		return ExitOK
	}
	a := &app{opts: opts, stdin: stdin, stdout: stdout, stderr: stderr}
	switch rest[0] {
	case "help":
		fmt.Fprint(stdout, usageText)
		return ExitOK
	case "version":
		return a.version()
	case "completion":
		return a.completion(rest[1:])
	case "man":
		fmt.Fprint(stdout, manPage)
		return ExitOK
	case "list":
		return a.withManager(func(m *core.Manager) int { return a.list(m, rest[1:]) })
	case "status":
		return a.withManager(func(m *core.Manager) int { return a.list(m, rest[1:]) })
	case "targets":
		return a.withManager(func(m *core.Manager) int { return a.targets(m) })
	case "doctor":
		return a.withManager(func(m *core.Manager) int { return a.doctor(m) })
	case "new":
		return a.withManager(func(m *core.Manager) int { return a.newSkill(m, rest[1:]) })
	case "enable":
		return a.withManager(func(m *core.Manager) int { return a.enable(m, rest[1:]) })
	case "disable":
		return a.withManager(func(m *core.Manager) int { return a.disable(m, rest[1:]) })
	case "sync":
		return a.withManager(func(m *core.Manager) int { return a.sync(m) })
	case "import":
		return a.withManager(func(m *core.Manager) int { return a.importCmd(m, rest[1:]) })
	case "export":
		return a.withManager(func(m *core.Manager) int { return a.export(m, rest[1:]) })
	case "backups":
		return a.withManager(func(m *core.Manager) int { return a.backups(m) })
	case "restore":
		return a.withManager(func(m *core.Manager) int { return a.restore(m, rest[1:]) })
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", rest[0], usageText)
		return ExitUsage
	}
}

type options struct {
	canonical string
	config    string
	json      bool
	dry       bool
	yes       bool
	verbose   bool
	quiet     bool
	help      bool
}

type app struct {
	opts   options
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasVal := arg, "", false
		if strings.HasPrefix(arg, "--") {
			if eq := strings.IndexByte(arg, '='); eq >= 0 {
				name, val, hasVal = arg[:eq], arg[eq+1:], true
			}
		}
		take := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s needs a value", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--help", "-h":
			opts.help = true
		case "--json":
			opts.json = true
		case "--dry-run":
			opts.dry = true
		case "--yes", "-y":
			opts.yes = true
		case "--verbose", "-v":
			opts.verbose = true
		case "--quiet", "-q":
			opts.quiet = true
		case "--canonical":
			v, err := take()
			if err != nil {
				return opts, nil, err
			}
			opts.canonical = v
		case "--config":
			v, err := take()
			if err != nil {
				return opts, nil, err
			}
			opts.config = v
		default:
			rest = append(rest, arg)
		}
	}
	return opts, rest, nil
}

func (a *app) withManager(fn func(*core.Manager) int) int {
	var opts []core.Option
	if a.opts.canonical != "" {
		opts = append(opts, core.WithCanonicalDir(a.opts.canonical))
	}
	if a.opts.config != "" {
		opts = append(opts, core.WithConfigDir(a.opts.config))
	}
	if a.opts.dry {
		opts = append(opts, core.WithDryRun(true))
	}
	if a.opts.verbose && !a.opts.quiet {
		opts = append(opts, core.WithLogger(slog.New(slog.NewTextHandler(a.stderr, nil))))
	}
	mgr, err := core.NewManager(opts...)
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	if !a.opts.quiet && !a.opts.json {
		for _, warning := range mgr.Warnings() {
			fmt.Fprintf(a.stderr, "warning: %s\n", warning)
		}
	}
	return fn(mgr)
}

func (a *app) version() int {
	if a.opts.json {
		return a.writeJSON(map[string]any{
			"schema_version": core.JSONSchemaVersion,
			"version":        version.Version,
			"commit":         version.Commit,
			"date":           version.Date,
		})
	}
	fmt.Fprintln(a.stdout, version.String())
	return ExitOK
}

func (a *app) list(m *core.Manager, args []string) int {
	skills, err := core.ListSkills(m.CanonicalDir)
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	type cell struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Detail string `json:"detail,omitempty"`
	}
	type row struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Targets     []cell `json:"targets"`
	}
	var rows []row
	ctx := context.Background()
	for _, skill := range skills {
		if filter != "" && skill.Name != filter {
			continue
		}
		item := row{Name: skill.Name, Description: skill.Description}
		for _, target := range m.GetTargets() {
			st, err := m.Status(ctx, skill.Name, target.ID)
			if err != nil {
				item.Targets = append(item.Targets, cell{ID: target.ID, Status: "error", Detail: err.Error()})
				continue
			}
			item.Targets = append(item.Targets, cell{ID: target.ID, Status: string(st.Code), Detail: st.Message})
		}
		rows = append(rows, item)
	}
	if rows == nil {
		rows = []row{}
	}
	if a.opts.json {
		return a.writeJSON(map[string]any{
			"schema_version": core.JSONSchemaVersion,
			"canonical":      m.CanonicalDir,
			"skills":         rows,
		})
	}
	if len(rows) == 0 {
		fmt.Fprintf(a.stdout, "No skills in %s\n", m.CanonicalDir)
		return ExitOK
	}
	for _, row := range rows {
		fmt.Fprintf(a.stdout, "%s\n", row.Name)
		if row.Description != "" {
			fmt.Fprintf(a.stdout, "  %s\n", row.Description)
		}
		for _, cell := range row.Targets {
			fmt.Fprintf(a.stdout, "  %s: %s\n", cell.ID, cell.Status)
		}
	}
	return ExitOK
}

func (a *app) targets(m *core.Manager) int {
	type row struct {
		ID       string `json:"id"`
		Label    string `json:"label"`
		Path     string `json:"path"`
		Expanded string `json:"expanded"`
		Mode     string `json:"mode"`
		Format   string `json:"format"`
		Builtin  bool   `json:"builtin"`
		Docs     string `json:"docs,omitempty"`
	}
	var rows []row
	for _, target := range m.GetTargets() {
		expanded, err := core.ExpandPath(target.Path)
		if err != nil {
			expanded = ""
		}
		rows = append(rows, row{
			ID: target.ID, Label: target.Label, Path: target.Path, Expanded: expanded,
			Mode: target.Mode, Format: target.Format, Builtin: target.Builtin, Docs: target.Docs,
		})
	}
	if a.opts.json {
		return a.writeJSON(map[string]any{"schema_version": core.JSONSchemaVersion, "targets": rows})
	}
	for _, row := range rows {
		fmt.Fprintf(a.stdout, "%s (%s)\n  path: %s\n  expanded: %s\n  mode: %s\n  format: %s\n", row.Label, row.ID, row.Path, row.Expanded, row.Mode, row.Format)
	}
	return ExitOK
}

func (a *app) doctor(m *core.Manager) int {
	report, err := m.Doctor(context.Background())
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	failed := false
	for _, check := range report.Checks {
		if check.Status == "fail" {
			failed = true
		}
	}
	if a.opts.json {
		if a.writeJSON(report) != ExitOK {
			return ExitError
		}
	} else {
		for _, check := range report.Checks {
			fmt.Fprintf(a.stdout, "[%s] %s: %s\n", check.Status, check.Name, check.Detail)
			if check.Fix != "" {
				fmt.Fprintf(a.stdout, "  fix: %s\n", check.Fix)
			}
		}
	}
	if failed {
		return ExitError
	}
	return ExitOK
}

func (a *app) newSkill(m *core.Manager, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(a.stderr, "usage: skills new <name> <description>")
		return ExitUsage
	}
	body := fmt.Sprintf("# %s\n\nInstructions for %s.\n", args[0], args[0])
	if a.opts.dry {
		fmt.Fprintf(a.stdout, "dry-run: would create %s\n", args[0])
		return ExitOK
	}
	skill, err := core.CreateSkill(m.CanonicalDir, args[0], strings.Join(args[1:], " "), body)
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	fmt.Fprintf(a.stdout, "created %s at %s\n", skill.Name, skill.Path)
	return ExitOK
}

func (a *app) enable(m *core.Manager, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(a.stderr, "usage: skills enable <skill> [target]")
		return ExitUsage
	}
	ctx := context.Background()
	targets := a.selected(m, args)
	if targets == nil {
		return ExitUsage
	}
	var errs []error
	ok := 0
	for _, target := range targets {
		res, err := m.Enable(ctx, args[0], target.ID)
		if err != nil {
			fmt.Fprintf(a.stderr, "error: %s: %v\n", target.ID, err)
			errs = append(errs, err)
			continue
		}
		ok++
		fmt.Fprintf(a.stdout, "%s: %s (%s)\n", target.ID, res.Message, res.TargetPath)
	}
	return summarize(ok, errs)
}

func (a *app) disable(m *core.Manager, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(a.stderr, "usage: skills disable <skill> [target]")
		return ExitUsage
	}
	targets := a.selected(m, args)
	if targets == nil {
		return ExitUsage
	}
	if !a.confirm(fmt.Sprintf("disable %s", args[0])) {
		return ExitUsage
	}
	ctx := context.Background()
	var errs []error
	ok := 0
	for _, target := range targets {
		if err := m.Disable(ctx, args[0], target.ID); err != nil {
			fmt.Fprintf(a.stderr, "error: %s: %v\n", target.ID, err)
			errs = append(errs, err)
			continue
		}
		ok++
		fmt.Fprintf(a.stdout, "disabled %s on %s\n", args[0], target.ID)
	}
	return summarize(ok, errs)
}

func (a *app) sync(m *core.Manager) int {
	report, err := m.Sync(context.Background())
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	if a.opts.json {
		return a.writeJSON(report)
	}
	if len(report.Updated) == 0 && len(report.Conflicts) == 0 && len(report.Errors) == 0 {
		fmt.Fprintln(a.stdout, "already up to date")
	}
	for _, line := range report.Updated {
		fmt.Fprintf(a.stdout, "updated %s\n", line)
	}
	for _, line := range report.Conflicts {
		fmt.Fprintf(a.stdout, "conflict %s\n", line)
	}
	for _, line := range report.Errors {
		fmt.Fprintf(a.stderr, "error: %s\n", line)
	}
	if len(report.Errors) > 0 && len(report.Updated) > 0 {
		return ExitPartial
	}
	if len(report.Errors) > 0 {
		return ExitError
	}
	if len(report.Conflicts) > 0 && len(report.Updated) > 0 {
		return ExitPartial
	}
	if len(report.Conflicts) > 0 {
		return ExitConflict
	}
	return ExitOK
}

func (a *app) importCmd(m *core.Manager, args []string) int {
	scanOnly := false
	all := false
	for _, arg := range args {
		switch arg {
		case "--scan":
			scanOnly = true
		case "--all":
			all = true
		default:
			fmt.Fprintf(a.stderr, "unknown import flag %s\n", arg)
			return ExitUsage
		}
	}
	cands, err := m.ScanImportCandidates(context.Background())
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	if cands == nil {
		cands = []core.ImportCandidate{}
	}
	if scanOnly || !all {
		if a.opts.json {
			return a.writeJSON(map[string]any{"schema_version": core.JSONSchemaVersion, "candidates": cands})
		}
		if len(cands) == 0 {
			fmt.Fprintln(a.stdout, "no untracked skills")
			return ExitOK
		}
		for _, c := range cands {
			fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", c.SkillName, c.TargetID, c.SourcePath)
		}
		if !all {
			fmt.Fprintln(a.stdout, "run skills import --all --yes to import them")
		}
		return ExitOK
	}
	if !a.confirm(fmt.Sprintf("import %d skill(s)", len(cands))) {
		return ExitUsage
	}
	if len(cands) == 0 {
		fmt.Fprintln(a.stdout, "no untracked skills")
		return ExitOK
	}
	imported, errs := m.ImportSelected(context.Background(), cands)
	if a.opts.json {
		messages := []string{}
		for _, err := range errs {
			messages = append(messages, err.Error())
		}
		if a.writeJSON(map[string]any{
			"schema_version": core.JSONSchemaVersion,
			"imported":       imported,
			"errors":         messages,
			"notes":          m.Notes(),
		}) != ExitOK {
			return ExitError
		}
	} else {
		for _, name := range imported {
			fmt.Fprintf(a.stdout, "imported %s\n", name)
		}
		for _, err := range errs {
			fmt.Fprintf(a.stderr, "error: %v\n", err)
		}
		for _, note := range m.Notes() {
			fmt.Fprintf(a.stderr, "warning: %s\n", note)
		}
	}
	return summarize(len(imported), errs)
}

func (a *app) export(m *core.Manager, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(a.stderr, "usage: skills export <file.zip>")
		return ExitUsage
	}
	if a.opts.dry {
		fmt.Fprintf(a.stdout, "dry-run: would export to %s\n", args[0])
		return ExitOK
	}
	f, err := os.OpenFile(args[0], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	defer f.Close()
	if err := m.ExportZip(context.Background(), nil, f); err != nil {
		_ = f.Close()
		_ = os.Remove(args[0])
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	fmt.Fprintf(a.stdout, "exported %s\n", args[0])
	return ExitOK
}

func (a *app) backups(m *core.Manager) int {
	list, err := m.ListBackups()
	if err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	if a.opts.json {
		return a.writeJSON(map[string]any{"schema_version": core.JSONSchemaVersion, "backups": list})
	}
	if len(list) == 0 {
		fmt.Fprintln(a.stdout, "no backups")
		return ExitOK
	}
	for _, b := range list {
		fmt.Fprintf(a.stdout, "%s\t%s\t%s\t%s\n", b.ID, b.Target, b.Skill, b.Kind)
	}
	return ExitOK
}

func (a *app) restore(m *core.Manager, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(a.stderr, "usage: skills restore <backup-id>")
		return ExitUsage
	}
	if !a.confirm("restore " + args[0]) {
		return ExitUsage
	}
	if err := m.Restore(context.Background(), args[0]); err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return codeOf(err)
	}
	fmt.Fprintf(a.stdout, "restored %s\n", args[0])
	return ExitOK
}

func (a *app) completion(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(a.stderr, "usage: skills completion bash|zsh|fish|pwsh")
		return ExitUsage
	}
	commands := "list status enable disable sync import export targets doctor new backups restore version completion man help"
	switch args[0] {
	case "bash":
		fmt.Fprintf(a.stdout, "complete -W %q skills\n", commands)
	case "zsh":
		fmt.Fprintf(a.stdout, "#compdef skills\ncompadd %s\n", commands)
	case "fish":
		fmt.Fprintf(a.stdout, "complete -c skills -f -a %q\n", commands)
	case "pwsh":
		fmt.Fprintf(a.stdout, "Register-ArgumentCompleter -CommandName skills -ScriptBlock { param($word) %q -split ' ' | Where-Object { $_ -like \"$word*\" } }\n", commands)
	default:
		fmt.Fprintf(a.stderr, "unknown shell %q\n", args[0])
		return ExitUsage
	}
	return ExitOK
}

func (a *app) selected(m *core.Manager, args []string) []core.Target {
	all := m.GetTargets()
	if len(args) < 2 {
		return all
	}
	for _, target := range all {
		if target.ID == args[1] {
			return []core.Target{target}
		}
	}
	fmt.Fprintf(a.stderr, "unknown target %q\n", args[1])
	return nil
}

func (a *app) confirm(action string) bool {
	if a.opts.dry || a.opts.yes {
		return true
	}
	if !isTerminal(a.stdin) {
		fmt.Fprintf(a.stderr, "refusing to %s without --yes\n", action)
		return false
	}
	fmt.Fprintf(a.stderr, "%s? type yes: ", action)
	var answer string
	_, _ = fmt.Fscanln(a.stdin, &answer)
	if answer != "yes" {
		fmt.Fprintln(a.stderr, "cancelled")
		return false
	}
	return true
}

func (a *app) writeJSON(v any) int {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(a.stderr, "error: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func summarize(ok int, errs []error) int {
	if len(errs) == 0 {
		return ExitOK
	}
	if ok > 0 {
		return ExitPartial
	}
	for _, err := range errs {
		if !errors.Is(err, core.ErrConflict) {
			return ExitError
		}
	}
	return ExitConflict
}

func codeOf(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, core.ErrConflict):
		return ExitConflict
	default:
		return ExitError
	}
}

func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

const usageText = `skills - manage AI agent skills

Usage:
  skills [--canonical path] [--config path] [--json] [--dry-run] [--yes] [--verbose] [--quiet] <command>

Commands:
  list [skill]              List skills and target status
  status [skill]            Same as list
  enable <skill> [target]   Deploy a skill
  disable <skill> [target]  Remove a deployment this tool created
  sync                      Update copies where only the canonical skill changed
  import --scan             List unmanaged skills in target directories
  import --all              Move those skills into the canonical store
  export <file.zip>         Write a zip of the canonical store
  new <name> <description>  Create a skill
  targets                   Show configured tools
  doctor                    Check directories, links, and state
  backups                   List retained copies
  restore <id>              Put a backup back at its original path
  version                   Print version, commit, and build date
  completion <shell>        Print a bash, zsh, fish, or pwsh completion script
  man                       Print a manual page
  help                      Show this text

Exit codes:
  0 success
  1 error
  2 usage
  3 conflict, nothing was overwritten
  4 partial success

--dry-run reports mutations without writing them. --yes confirms disable, restore, and import --all. NO_COLOR is honored because this command prints no color.
This tool never runs scripts stored inside a skill.
`

const manPage = `.TH SKILLS 1
.SH NAME
skills \- manage AI agent skills across coding tools
.SH SYNOPSIS
.B skills
[\fIoptions\fR]
.I command
.SH DESCRIPTION
Skills Manager keeps one canonical store of SKILL.md folders and deploys them to Claude Code, Codex CLI, OpenCode, and Gemini CLI. It deletes a path only when that path is a symlink to the canonical skill or a copy recorded in state.json.
.SH OPTIONS
.TP
.B \-\-dry-run
Do not write.
.TP
.B \-\-json
Print a versioned JSON document.
.TP
.B \-\-yes
Confirm a destructive command.
.SH EXIT STATUS
0 success, 1 error, 2 usage, 3 conflict, 4 partial success.
.SH SEE ALSO
skills doctor
`
