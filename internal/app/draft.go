package app

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

func draftSpec(brief string, catalog []ToolSpec, explicitName string, maxTools int) SkillSpec {
	lines := compactLines(brief)
	name := strings.TrimSpace(explicitName)
	if name == "" {
		name = inferName(lines)
	}
	description := inferDescription(lines, name)
	tags := inferTags(brief)
	category := inferCategory(brief, tags)
	tools := selectTools(brief, catalog, maxTools)
	if len(tools) == 0 {
		tools = []ToolSpec{placeholderTool()}
	}

	spec := SkillSpec{
		Name:        name,
		Slug:        slugify(name),
		Description: description,
		Audience:    inferAudience(lines, category),
		Category:    category,
		Tags:        tags,
		Triggers:    inferTriggers(brief, tags),
		Constraints: inferConstraints(brief),
		Checks:      inferChecks(category),
		Examples:    inferExamples(lines, name, category),
		Workflow:    inferWorkflow(category, tools),
		Tools:       tools,
	}

	if err := validateSpec(spec); err != nil {
		return SkillSpec{
			Name:        name,
			Slug:        slugify(name),
			Description: description,
			Tools:       tools,
		}
	}
	return spec
}

func compactLines(body string) []string {
	raw := strings.Split(body, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func inferName(lines []string) string {
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" {
			continue
		}
		if !strings.Contains(line, ":") && !strings.HasSuffix(line, ".") && wordCount(line) <= 8 {
			return strings.TrimSpace(line)
		}
		break
	}
	return "Generated Skill"
}

func inferDescription(lines []string, name string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if trimmed == "" || strings.EqualFold(trimmed, name) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "audience:") {
			continue
		}
		return trimmed
	}
	return "Coordinate tools and workflows for focused agent tasks."
}

func inferAudience(lines []string, category string) string {
	for _, line := range lines {
		if value, ok := cutPrefixedValue(line, "audience:"); ok {
			return value
		}
	}

	switch category {
	case "retrieval-evaluation":
		return "Applied AI researchers and retrieval engineers."
	case "observability":
		return "Agent platform engineers and incident responders."
	case "dataset-quality":
		return "ML data engineers and evaluation owners."
	default:
		return "Agent teams that want repeatable workflows."
	}
}

func inferTags(brief string) []string {
	type keywordTag struct {
		keyword string
		tag     string
	}

	keywords := []keywordTag{
		{"retrieval", "retrieval"},
		{"rag", "rag"},
		{"dataset", "dataset"},
		{"evaluation", "evaluation"},
		{"prompt", "prompting"},
		{"trace", "traces"},
		{"observability", "observability"},
		{"incident", "reliability"},
		{"benchmark", "benchmarking"},
		{"tool", "tooling"},
		{"policy", "guardrails"},
		{"research", "research"},
	}

	brief = strings.ToLower(brief)
	seen := map[string]struct{}{}
	var tags []string
	for _, item := range keywords {
		if strings.Contains(brief, item.keyword) {
			if _, ok := seen[item.tag]; ok {
				continue
			}
			seen[item.tag] = struct{}{}
			tags = append(tags, item.tag)
		}
	}

	if len(tags) == 0 {
		tags = []string{"workflow", "agents"}
	}
	sort.Strings(tags)
	return tags
}

func inferCategory(brief string, tags []string) string {
	body := strings.ToLower(brief + " " + strings.Join(tags, " "))
	switch {
	case strings.Contains(body, "retrieval") || strings.Contains(body, "rag"):
		return "retrieval-evaluation"
	case strings.Contains(body, "dataset"):
		return "dataset-quality"
	case strings.Contains(body, "trace") || strings.Contains(body, "incident") || strings.Contains(body, "observability"):
		return "observability"
	case strings.Contains(body, "prompt"):
		return "prompt-optimization"
	default:
		return "agent-workflow"
	}
}

func inferTriggers(brief string, tags []string) []string {
	generic := []string{
		"You need a consistent workflow instead of one-off prompts.",
		"You want reproducible output that can be validated before shipping.",
	}

	// Category-specific triggers come first: init turns the first triggers
	// into the "Use when ..." clause of the SKILL.md description, and the
	// specific ones are the more useful text for an agent deciding to load it.
	var specific []string
	body := strings.ToLower(brief)
	switch {
	case strings.Contains(body, "retrieval") || containsTag(tags, "retrieval"):
		specific = append(specific,
			"You are comparing retrieval runs, benchmark regressions, or miss patterns.",
			"You need a short diagnosis of why a RAG system degraded.")
	case strings.Contains(body, "dataset") || containsTag(tags, "dataset"):
		specific = append(specific,
			"You are preparing train or eval data and want to catch quality issues early.")
	case strings.Contains(body, "prompt") || containsTag(tags, "prompting"):
		specific = append(specific,
			"You are testing prompt variants and need a controlled prompt pack.")
	}

	return uniqueNonEmpty(append(specific, generic...))
}

func inferConstraints(brief string) []string {
	constraints := []string{
		"Prefer deterministic and inspectable tool outputs over free-form speculation.",
		"Surface missing inputs or blockers before taking expensive actions.",
	}
	body := strings.ToLower(brief)
	if strings.Contains(body, "offline") {
		constraints = append(constraints, "Stay offline unless the user explicitly asks for external calls.")
	}
	if strings.Contains(body, "production") || strings.Contains(body, "incident") {
		constraints = append(constraints, "Call out user-facing risk, regressions, and validation gaps explicitly.")
	}
	return uniqueNonEmpty(constraints)
}

func inferChecks(category string) []string {
	checks := []string{
		"Confirm the required files, paths, or inputs before running tools.",
		"Summarize the outcome with concrete next steps instead of raw logs alone.",
	}

	switch category {
	case "retrieval-evaluation":
		checks = append(checks,
			"Report the highest-impact misses or regressions with supporting evidence.",
			"Separate retrieval failures from generation failures when both are possible.")
	case "dataset-quality":
		checks = append(checks,
			"Highlight leakage, duplicates, or label conflicts before discussing aggregate metrics.")
	case "observability":
		checks = append(checks,
			"Call out flaky tools, dominant failure signatures, and likely root causes.")
	}
	return uniqueNonEmpty(checks)
}

func inferExamples(lines []string, name string, category string) []string {
	var examples []string
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			examples = append(examples, strings.TrimSpace(line[2:]))
		}
	}
	if len(examples) > 0 {
		return examples
	}

	switch category {
	case "retrieval-evaluation":
		return []string{
			"Compare the last two retrieval runs and explain the ranking regression.",
			"Summarize the highest-impact misses in this RAG benchmark.",
		}
	case "dataset-quality":
		return []string{
			"Scan this train/eval pair and call out duplication, leakage, and label issues.",
			"Summarize the dataset quality risks before we start training.",
		}
	case "observability":
		return []string{
			"Analyze this agent trace and identify the slowest or flakiest tool path.",
			"Summarize the dominant failure mode and the next debugging step.",
		}
	default:
		return []string{
			"Use " + name + " to inspect the problem and propose a concrete next step.",
		}
	}
}

func inferWorkflow(category string, tools []ToolSpec) []WorkflowStep {
	steps := []WorkflowStep{
		{Name: "Inspect Inputs", Goal: "Validate the request, available files, and task constraints."},
		{Name: "Execute Tools", Goal: "Run the most relevant tools and collect evidence."},
		{Name: "Synthesize Output", Goal: "Summarize findings, risks, and next actions."},
	}

	if category == "retrieval-evaluation" {
		steps[1].Goal = "Run retrieval or scoring tools, then inspect failures and misses."
	}
	if category == "observability" {
		steps[1].Goal = "Analyze traces, outliers, and failure clusters before proposing root causes."
	}
	if len(tools) > 0 {
		steps = append(steps, WorkflowStep{
			Name: "Verify Tool Coverage",
			Goal: fmt.Sprintf("Make sure the selected workflow uses the right tool set, starting with `%s`.", tools[0].Name),
		})
	}
	return steps
}

func selectTools(brief string, catalog []ToolSpec, maxTools int) []ToolSpec {
	if len(catalog) == 0 {
		return nil
	}
	if maxTools <= 0 {
		maxTools = 3
	}

	queryTokens := tokenCounts(brief)
	type scoredTool struct {
		tool  ToolSpec
		score int
	}
	scored := make([]scoredTool, 0, len(catalog))
	for _, tool := range catalog {
		score := 0
		nameTokens := tokenCounts(tool.Name)
		descTokens := tokenCounts(tool.Description + " " + tool.Example + " " + tool.Command + " " + strings.Join(tool.Inputs, " ") + " " + strings.Join(tool.Outputs, " "))
		score += 3 * overlapScore(queryTokens, nameTokens)
		score += 2 * overlapScore(queryTokens, descTokens)
		scored = append(scored, scoredTool{tool: tool, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].tool.Name < scored[j].tool.Name
		}
		return scored[i].score > scored[j].score
	})

	var selected []ToolSpec
	for _, item := range scored {
		if len(selected) >= maxTools {
			break
		}
		if item.score == 0 && len(selected) > 0 {
			break
		}
		selected = append(selected, item.tool)
	}
	if len(selected) == 0 {
		limit := min(maxTools, len(scored))
		for i := 0; i < limit; i++ {
			selected = append(selected, scored[i].tool)
		}
	}
	return selected
}

func placeholderTool() ToolSpec {
	return ToolSpec{
		Name:        "replace-me",
		Command:     "echo 'replace me with a real tool'",
		Description: "Placeholder tool generated because no tool catalog was provided.",
		Example:     "echo 'replace me with a real tool'",
	}
}

func cutPrefixedValue(line string, prefix string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(line), prefix) {
		return "", false
	}
	return strings.TrimSpace(line[len(prefix):]), true
}

func tokenCounts(text string) map[string]int {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, text)

	counts := map[string]int{}
	for _, token := range strings.Fields(normalized) {
		if len(token) < 2 {
			continue
		}
		counts[token]++
	}
	return counts
}

func overlapScore(a map[string]int, b map[string]int) int {
	score := 0
	for token, left := range a {
		if right, ok := b[token]; ok {
			score += min(left, right)
		}
	}
	return score
}

func slugify(name string) string {
	var builder strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash && builder.Len() > 0 {
			builder.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func containsTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
}

func uniqueNonEmpty(items []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func wordCount(text string) int {
	return len(strings.Fields(text))
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
