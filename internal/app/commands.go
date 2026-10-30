package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// parseFlags parses args into fs. Help requests (-h, -help, --help) print the
// flag usage to stdout and report help=true; parse errors are written to
// stderr. Callers return 0 for help and 2 for any other error.
func parseFlags(fs *flag.FlagSet, usageLine string, args []string) (help bool, err error) {
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	fs.Usage = func() {
		fmt.Fprintf(&buf, "Usage: %s\n\nFlags:\n", usageLine)
		fs.PrintDefaults()
	}
	err = fs.Parse(args)
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, flag.ErrHelp):
		_, _ = buf.WriteTo(os.Stdout)
		return true, nil
	default:
		_, _ = buf.WriteTo(os.Stderr)
		return false, err
	}
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	specPath := fs.String("spec", "", "path to a skill spec JSON file")
	outDir := fs.String("out", "", "directory to create")
	force := fs.Bool("force", false, "overwrite an existing output directory")

	if help, err := parseFlags(fs, "skillforge init -spec FILE -out DIR [-force]", args); err != nil {
		return 2
	} else if help {
		return 0
	}
	if *specPath == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "both -spec and -out are required")
		return 2
	}

	spec, err := loadSpec(*specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err := scaffold(spec, *outDir, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("generated skill scaffold in %s\n", *outDir)
	_, notes := buildDescription(spec)
	for _, note := range notes {
		fmt.Fprintln(os.Stderr, note)
	}
	if warning := folderNameWarning(spec, *outDir); warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	return 0
}

// folderNameWarning returns a one-line warning when the output folder name
// differs from the skill name, since Claude Code and Codex expect them equal.
func folderNameWarning(spec SkillSpec, outDir string) string {
	name := skillName(spec)
	folder := filepath.Base(filepath.Clean(outDir))
	if abs, err := filepath.Abs(outDir); err == nil {
		folder = filepath.Base(abs)
	}
	if folder == name {
		return ""
	}
	return fmt.Sprintf("warning: output folder %q does not match skill name %q; Claude Code and Codex expect the folder name to equal the SKILL.md name (rename the folder or use -out .../%s)", folder, name, name)
}

const (
	draftReviewNote          = "review: triggers, constraints, checks and workflow are generated from category defaults; edit them before init"
	draftNoSlugNote          = "review: the name has no ASCII letters or digits, so no slug was generated; set an ASCII `slug` in the spec before init"
	draftPlaceholderToolNote = "review: no tool was selected from a catalog; the spec contains the placeholder tool \"replace-me\", replace it before init"
)

func runDraft(args []string) int {
	fs := flag.NewFlagSet("draft", flag.ContinueOnError)
	briefPath := fs.String("brief", "", "path to a natural-language brief")
	catalogPath := fs.String("catalog", "", "optional path to a JSON tool catalog")
	outPath := fs.String("out", "", "optional output JSON path")
	name := fs.String("name", "", "optional explicit skill name")
	maxTools := fs.Int("max-tools", 3, "maximum number of tools to retrieve from the catalog")

	if help, err := parseFlags(fs, "skillforge draft -brief FILE [-catalog FILE] [-out FILE] [-name NAME] [-max-tools N]", args); err != nil {
		return 2
	} else if help {
		return 0
	}
	if *briefPath == "" {
		fmt.Fprintln(os.Stderr, "-brief is required")
		return 2
	}

	brief, err := os.ReadFile(*briefPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("read brief: %w", err))
		return 1
	}

	var catalog []ToolSpec
	if *catalogPath != "" {
		catalog, err = loadToolCatalog(*catalogPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	spec := draftSpec(string(brief), catalog, *name, *maxTools)
	fmt.Fprintln(os.Stderr, draftReviewNote)
	if skillName(spec) == "" {
		fmt.Fprintln(os.Stderr, draftNoSlugNote)
	}
	if len(spec.Tools) == 1 && spec.Tools[0].Name == placeholderTool().Name {
		fmt.Fprintln(os.Stderr, draftPlaceholderToolNote)
	}
	body, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("encode drafted spec: %w", err))
		return 1
	}

	if *outPath == "" {
		fmt.Println(string(body))
		return 0
	}

	if err := os.WriteFile(*outPath, append(body, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("write drafted spec: %w", err))
		return 1
	}
	fmt.Printf("wrote drafted spec to %s\n", *outPath)
	return 0
}
