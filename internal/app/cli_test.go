package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests swap os.Stdout/os.Stderr, so none of them call t.Parallel.

func runCapturing(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()

	code = Run(args)

	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outW.Close()
	_ = errW.Close()
	read := func(f *os.File) string {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		_ = f.Close()
		return sb.String()
	}
	return code, read(outR), read(errR)
}

func TestHelpAndVersionExitZero(t *testing.T) {
	cases := [][]string{
		{"-h"}, {"--help"}, {"-help"}, {"help"},
		{"help", "init"}, {"help", "draft"}, {"help", "tui"},
		{"init", "-h"}, {"init", "--help"},
		{"draft", "-h"}, {"draft", "--help"},
		{"tui", "--help"},
		{"version"}, {"--version"},
	}
	for _, args := range cases {
		code, stdout, stderr := runCapturing(t, args...)
		if code != 0 {
			t.Errorf("%v: exit code = %d, want 0 (stderr %q)", args, code, stderr)
		}
		if strings.TrimSpace(stdout) == "" {
			t.Errorf("%v: expected output on stdout", args)
		}
		if stderr != "" {
			t.Errorf("%v: unexpected stderr %q", args, stderr)
		}
	}
}

func TestHelpContent(t *testing.T) {
	_, stdout, _ := runCapturing(t, "--help")
	for _, want := range []string{"skillforge init", "skillforge draft", "must not"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("top-level help is missing %q:\n%s", want, stdout)
		}
	}

	_, stdout, _ = runCapturing(t, "init", "-h")
	for _, want := range []string{"Usage: skillforge init", "-spec", "-out", "-force"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("init help is missing %q:\n%s", want, stdout)
		}
	}

	_, stdout, _ = runCapturing(t, "draft", "--help")
	for _, want := range []string{"Usage: skillforge draft", "-brief", "-catalog", "-max-tools"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("draft help is missing %q:\n%s", want, stdout)
		}
	}

	_, stdout, _ = runCapturing(t, "version")
	if !strings.HasPrefix(stdout, "skillforge ") {
		t.Errorf("version output = %q", stdout)
	}
}

func TestVersionUsesInjectedVersion(t *testing.T) {
	old := Version
	Version = "1.2.3"
	defer func() { Version = old }()
	if _, stdout, _ := runCapturing(t, "--version"); strings.TrimSpace(stdout) != "skillforge 1.2.3" {
		t.Errorf("version output = %q", stdout)
	}
}

func TestRealErrorsKeepExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"bogus"}, 2},
		{[]string{"help", "bogus"}, 2},
		{[]string{"init"}, 2},
		{[]string{"init", "-spec", "x"}, 2},
		{[]string{"init", "-bogus"}, 2},
		{[]string{"draft"}, 2},
		{[]string{"draft", "-bogus"}, 2},
		{[]string{"init", "-spec", filepath.Join(t.TempDir(), "missing.json"), "-out", filepath.Join(t.TempDir(), "o")}, 1},
		{[]string{"draft", "-brief", filepath.Join(t.TempDir(), "missing.md")}, 1},
	}
	for _, tc := range cases {
		code, stdout, stderr := runCapturing(t, tc.args...)
		if code != tc.want {
			t.Errorf("%v: exit code = %d, want %d", tc.args, code, tc.want)
		}
		if stdout != "" && tc.want == 2 && tc.args[0] != "init" && tc.args[0] != "draft" {
			t.Errorf("%v: usage errors should not print to stdout, got %q", tc.args, stdout)
		}
		if stderr == "" {
			t.Errorf("%v: expected an error message on stderr", tc.args)
		}
	}
}

func writeSpecFile(t *testing.T, dir string, spec SkillSpec) string {
	t.Helper()
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInitWarnsWhenFolderDiffersFromSkillName(t *testing.T) {
	dir := t.TempDir()
	spec := SkillSpec{
		Name:        "Research Ops",
		Description: "Analyze notes.",
		Tools:       []ToolSpec{{Name: "t", Command: "t", Description: "d"}},
	}
	specPath := writeSpecFile(t, dir, spec)

	code, stdout, stderr := runCapturing(t, "init", "-spec", specPath, "-out", filepath.Join(dir, "other"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (warning must not change exit codes); stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "generated skill scaffold") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.HasPrefix(stderr, "warning:") || strings.Count(strings.TrimSpace(stderr), "\n") != 0 ||
		!strings.Contains(stderr, `"other"`) || !strings.Contains(stderr, `"research-ops"`) {
		t.Errorf("expected a one-line folder name warning, got %q", stderr)
	}

	code, _, stderr = runCapturing(t, "init", "-spec", specPath, "-out", filepath.Join(dir, "research-ops"))
	if code != 0 || stderr != "" {
		t.Errorf("matching folder: code %d, stderr %q; want 0 and no warning", code, stderr)
	}
}

func TestDraftPrintsReviewNoteAndKeepsSpecSchema(t *testing.T) {
	dir := t.TempDir()
	brief := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(brief, []byte("# Retrieval Evaluator\nCompare retrieval runs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCapturing(t, "draft", "-brief", brief)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, "review: triggers, constraints, checks and workflow are generated from category defaults; edit them before init") {
		t.Errorf("missing review note on stderr: %q", stderr)
	}
	if !strings.Contains(stderr, "replace-me") {
		t.Errorf("missing placeholder tool note on stderr: %q", stderr)
	}
	// stdout is still only the spec JSON.
	var spec SkillSpec
	if err := json.Unmarshal([]byte(stdout), &spec); err != nil {
		t.Fatalf("stdout is not a spec: %v\n%s", err, stdout)
	}
	var generic map[string]any
	if err := json.Unmarshal([]byte(stdout), &generic); err != nil {
		t.Fatal(err)
	}
	for key := range generic {
		switch key {
		case "name", "slug", "description", "audience", "category", "tags", "triggers", "constraints", "checks", "examples", "workflow", "tools":
		default:
			t.Errorf("unexpected new spec field %q", key)
		}
	}
}
