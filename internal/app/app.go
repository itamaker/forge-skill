package app

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Version is the release version. main sets it (GoReleaser injects
// main.version); "dev" falls back to the module version from build info.
var Version = "dev"

func Run(args []string) int {
	if len(args) == 0 {
		return runTUI()
	}

	// "help" and "version" directly after a subcommand behave like the flags.
	if len(args) > 1 && (args[0] == "init" || args[0] == "draft" || args[0] == "tui" || args[0] == "interactive") {
		switch {
		case args[1] == "help":
			return runHelp([]string{args[0]})
		case isVersionArg(args[1]):
			return runVersion()
		}
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "draft":
		return runDraft(args[1:])
	case "tui", "interactive":
		if len(args) > 1 && isHelpArg(args[1]) {
			return runHelp([]string{"tui"})
		}
		return runTUI()
	case "help", "-h", "-help", "--help":
		return runHelp(args[1:])
	case "version", "-version", "--version":
		return runVersion()
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

// runHelp prints top-level usage, or the usage of the named subcommand.
func runHelp(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, usageText)
		return 0
	}
	switch args[0] {
	case "init":
		return runInit([]string{"-h"})
	case "draft":
		return runDraft([]string{"-h"})
	case "tui", "interactive":
		fmt.Fprint(os.Stdout, tuiUsageText)
		return 0
	case "help", "version", "-h", "-help", "--help":
		fmt.Fprint(os.Stdout, usageText)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

func runVersion() int {
	fmt.Fprintln(os.Stdout, "skillforge "+versionString())
	return 0
}

func isVersionArg(arg string) bool {
	switch arg {
	case "version", "-version", "--version":
		return true
	}
	return false
}

func isHelpArg(arg string) bool {
	switch arg {
	case "-h", "-help", "--help":
		return true
	}
	return false
}

func versionString() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

const usageText = `skillforge scaffolds agent skill folders (SKILL.md with YAML frontmatter,
agents/openai.yaml, manifest.json) from a JSON spec, and drafts specs from briefs.

Usage:
  skillforge init  -spec FILE -out DIR [-force]
  skillforge draft -brief FILE [-catalog FILE] [-out FILE] [-name NAME] [-max-tools N]
  skillforge help [init|draft|tui]
  skillforge version

Examples:
  skillforge draft -brief examples/brief.md -catalog examples/tools.json -out /tmp/spec.json
  skillforge init -spec /tmp/spec.json -out /tmp/retrieval-evaluator

Notes:
  -out for init is the skill folder; name it after the skill slug (the "name"
  in SKILL.md) because Claude Code and Codex expect folder name == name.
  draft fills triggers, constraints, checks and workflow from category
  defaults; edit the spec before running init.

  Running skillforge with no arguments (or "tui") launches an interactive
  terminal UI. Scripts and coding agents must not do that: use init and draft.

Exit codes: 0 success, 1 failure, 2 usage error.
`

const tuiUsageText = `Usage: skillforge [tui]

Launches the interactive terminal UI. It needs a real terminal; scripts and
coding agents should use "skillforge init" and "skillforge draft" instead.
`
