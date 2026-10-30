package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func scaffold(spec SkillSpec, outDir string, force bool) error {
	if info, err := os.Stat(outDir); err == nil {
		if !force {
			return fmt.Errorf("output directory %s already exists; use -force to overwrite", outDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("output path %s is not a directory", outDir)
		}
		if err := os.RemoveAll(outDir); err != nil {
			return fmt.Errorf("clear output directory: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect output directory: %w", err)
	}

	paths := []string{
		outDir,
		filepath.Join(outDir, "agents"),
		filepath.Join(outDir, "bin"),
		filepath.Join(outDir, "examples"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", path, err)
		}
	}

	// The slug doubles as the Agent Skills `name`, so keep it valid.
	spec.Slug = skillName(spec)

	manifest, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	files := map[string]string{
		filepath.Join(outDir, "SKILL.md"):              renderSkillMarkdown(spec),
		filepath.Join(outDir, "agents", "openai.yaml"): renderOpenAIYaml(spec),
		filepath.Join(outDir, "manifest.json"):         string(manifest) + "\n",
		filepath.Join(outDir, "bin", "README.md"):      renderBinReadme(spec),
		filepath.Join(outDir, "examples", "usage.md"):  renderUsageGuide(spec),
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return nil
}

func renderSkillMarkdown(spec SkillSpec) string {
	var builder strings.Builder
	builder.WriteString(renderFrontmatter(spec))
	builder.WriteString("# " + spec.Name + "\n\n")
	builder.WriteString(spec.Description + "\n\n")

	if spec.Slug != "" || spec.Category != "" || len(spec.Tags) > 0 {
		builder.WriteString("## Metadata\n\n")
		if spec.Slug != "" {
			builder.WriteString("- Slug: `" + spec.Slug + "`\n")
		}
		if spec.Category != "" {
			builder.WriteString("- Category: `" + spec.Category + "`\n")
		}
		if len(spec.Tags) > 0 {
			builder.WriteString("- Tags: `" + strings.Join(spec.Tags, "`, `") + "`\n")
		}
		builder.WriteString("\n")
	}

	if spec.Audience != "" {
		builder.WriteString("## Audience\n\n")
		builder.WriteString(spec.Audience + "\n\n")
	}

	if len(spec.Triggers) > 0 {
		builder.WriteString("## When To Use\n\n")
		for _, trigger := range spec.Triggers {
			builder.WriteString("- " + trigger + "\n")
		}
		builder.WriteString("\n")
	}

	if len(spec.Constraints) > 0 {
		builder.WriteString("## Constraints\n\n")
		for _, constraint := range spec.Constraints {
			builder.WriteString("- " + constraint + "\n")
		}
		builder.WriteString("\n")
	}

	if len(spec.Workflow) > 0 {
		builder.WriteString("## Workflow\n\n")
		for i, step := range spec.Workflow {
			builder.WriteString(fmt.Sprintf("%d. **%s**: %s\n", i+1, step.Name, step.Goal))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Tools\n\n")
	for _, tool := range spec.Tools {
		builder.WriteString("- `" + tool.Name + "`: " + tool.Description + "\n")
		builder.WriteString("  Command: `" + tool.Command + "`\n")
		if len(tool.Inputs) > 0 {
			builder.WriteString("  Inputs: `" + strings.Join(tool.Inputs, "`, `") + "`\n")
		}
		if len(tool.Outputs) > 0 {
			builder.WriteString("  Outputs: `" + strings.Join(tool.Outputs, "`, `") + "`\n")
		}
		if tool.Example != "" {
			builder.WriteString("  Example: `" + tool.Example + "`\n")
		}
	}

	if len(spec.Examples) > 0 {
		builder.WriteString("\n## Example Prompts\n\n")
		for _, example := range spec.Examples {
			builder.WriteString("- " + example + "\n")
		}
	}

	if len(spec.Checks) > 0 {
		builder.WriteString("\n## Validation\n\n")
		for _, check := range spec.Checks {
			builder.WriteString("- " + check + "\n")
		}
	}

	builder.WriteString("\n## Usage\n\n")
	builder.WriteString("Place this directory in your agent's skills folder and keep the folder name equal to the `name` in the frontmatter above (`" + skillName(spec) + "`). Claude Code and Codex load `SKILL.md` from there.\n")
	return builder.String()
}

func renderBinReadme(spec SkillSpec) string {
	var builder strings.Builder
	builder.WriteString("# Tool Commands\n\n")
	builder.WriteString("This folder documents the executable commands expected by the generated skill.\n\n")
	for _, tool := range spec.Tools {
		builder.WriteString("## " + tool.Name + "\n\n")
		builder.WriteString("- Command: `" + tool.Command + "`\n")
		builder.WriteString("- Purpose: " + tool.Description + "\n")
		if len(tool.Inputs) > 0 {
			builder.WriteString("- Inputs: `" + strings.Join(tool.Inputs, "`, `") + "`\n")
		}
		if len(tool.Outputs) > 0 {
			builder.WriteString("- Outputs: `" + strings.Join(tool.Outputs, "`, `") + "`\n")
		}
		if tool.Example != "" {
			builder.WriteString("- Example: `" + tool.Example + "`\n")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderUsageGuide(spec SkillSpec) string {
	var builder strings.Builder
	builder.WriteString("# Usage Guide\n\n")
	builder.WriteString("Suggested user prompts for the generated skill:\n\n")
	for _, example := range spec.Examples {
		builder.WriteString("- " + example + "\n")
	}
	if len(spec.Checks) > 0 {
		builder.WriteString("\nSuggested validation checklist:\n\n")
		for _, check := range spec.Checks {
			builder.WriteString("- " + check + "\n")
		}
	}
	return builder.String()
}
