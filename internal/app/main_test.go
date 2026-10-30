package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldCreatesExpectedFiles(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "Research Assistant",
		Description: "Summarize papers and search local notes.",
		Audience:    "Applied AI teams.",
		Examples:    []string{"Summarize the latest retrieval notes."},
		Tools: []ToolSpec{
			{
				Name:        "paper-search",
				Command:     "paper-search --query '{{query}}'",
				Description: "Search the local paper index.",
				Example:     "paper-search --query 'rag evaluation'",
			},
		},
	}

	outDir := filepath.Join(t.TempDir(), "research-skill")
	if err := scaffold(spec, outDir, false); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}

	skillBody, err := os.ReadFile(filepath.Join(outDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}

	if !strings.Contains(string(skillBody), "paper-search") {
		t.Fatalf("SKILL.md did not include generated tool documentation")
	}

	if _, err := os.Stat(filepath.Join(outDir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json missing: %v", err)
	}
}

func TestDraftSpecRetrievesRelevantTools(t *testing.T) {
	t.Parallel()

	catalog := []ToolSpec{
		{
			Name:        "retrieval-score",
			Command:     "retrieval-score --run run.json --qrels qrels.json",
			Description: "Score retrieval benchmarks and analyze misses.",
		},
		{
			Name:        "trace-scan",
			Command:     "trace-scan --input run.jsonl",
			Description: "Scan agent traces for flaky execution paths.",
		},
	}

	spec := draftSpec(strings.Join([]string{
		"# Retrieval Evaluator",
		"Build a skill for retrieval benchmark analysis and miss diagnosis.",
		"Audience: Applied researchers.",
		"- Compare two retrieval runs and explain the regression.",
	}, "\n"), catalog, "", 1)

	if spec.Name != "Retrieval Evaluator" {
		t.Fatalf("Name = %q, want Retrieval Evaluator", spec.Name)
	}
	if spec.Category != "retrieval-evaluation" {
		t.Fatalf("Category = %q, want retrieval-evaluation", spec.Category)
	}
	if len(spec.Tools) != 1 || spec.Tools[0].Name != "retrieval-score" {
		t.Fatalf("draft selected %+v, want retrieval-score", spec.Tools)
	}
	if len(spec.Workflow) == 0 {
		t.Fatalf("expected generated workflow")
	}
}

func TestLoadToolCatalogWrappedShape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tools.json")
	body, err := json.Marshal(map[string]any{
		"tools": []map[string]any{
			{
				"name":        "dataset-scan",
				"command":     "dataset-scan --train train.jsonl",
				"description": "Check dataset quality.",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	tools, err := loadToolCatalog(path)
	if err != nil {
		t.Fatalf("loadToolCatalog() error = %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "dataset-scan" {
		t.Fatalf("tools = %+v, want dataset-scan", tools)
	}
}

func TestDraftTriggersListSpecificBeforeGeneric(t *testing.T) {
	t.Parallel()

	spec := draftSpec("# Retrieval Evaluator\nCompare retrieval runs.\n", nil, "", 1)
	if len(spec.Triggers) < 3 || !strings.Contains(spec.Triggers[0], "retrieval runs") {
		t.Fatalf("Triggers = %q, want the retrieval-specific trigger first", spec.Triggers)
	}
	if !strings.Contains(spec.Triggers[len(spec.Triggers)-1], "reproducible output") {
		t.Fatalf("Triggers = %q, want generic triggers last", spec.Triggers)
	}
}
