package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var validSkillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// parseFrontmatter reads the frontmatter written by renderFrontmatter. Values
// are either bare words (name) or JSON/YAML double-quoted strings.
func parseFrontmatter(t *testing.T, doc string) map[string]string {
	t.Helper()
	if !strings.HasPrefix(doc, "---\n") {
		t.Fatalf("document does not start with frontmatter:\n%s", firstLines(doc, 6))
	}
	end := strings.Index(doc[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("frontmatter is not closed:\n%s", firstLines(doc, 6))
	}
	fields := map[string]string{}
	for _, line := range strings.Split(doc[4:4+end], "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("frontmatter line %q is not key: value", line)
		}
		if strings.HasPrefix(value, `"`) {
			var decoded string
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				t.Fatalf("frontmatter value %q is not a valid quoted scalar: %v", value, err)
			}
			value = decoded
		}
		fields[key] = value
	}
	return fields
}

func firstLines(text string, n int) string {
	lines := strings.SplitN(text, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func TestSkillNameSanitizes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"retrieval-evaluator", "retrieval-evaluator"},
		{"Retrieval Evaluator", "retrieval-evaluator"},
		{"  --Foo__Bar!!  ", "foo-bar"},
		{"a--b---c", "a-b-c"},
		{"检索评估", ""},
		{"Café Tool", "caf-tool"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := sanitizeSkillName(tc.in); got != tc.want {
			t.Errorf("sanitizeSkillName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	long := strings.Repeat("abcd-", 30)
	got := sanitizeSkillName(long)
	if len(got) > maxSkillNameLen || !validSkillName.MatchString(got) {
		t.Errorf("long name sanitized to %q (len %d), want valid and <= %d", got, len(got), maxSkillNameLen)
	}

	if got := skillName(SkillSpec{Name: "检索评估"}); got != fallbackSkillName {
		t.Errorf("skillName with unusable name = %q, want %q", got, fallbackSkillName)
	}
	if got := skillName(SkillSpec{Name: "Display Name", Slug: "Bad Slug!"}); got != "bad-slug" {
		t.Errorf("skillName should sanitize the slug, got %q", got)
	}
	if got := skillName(SkillSpec{Name: "Display Name"}); got != "display-name" {
		t.Errorf("skillName should fall back to the name, got %q", got)
	}
}

func TestFrontmatterNameQuotingOnlyWhenAmbiguous(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"research-ops": "research-ops",
		"2024-report":  `"2024-report"`,
		"yes":          `"yes"`,
		"null":         `"null"`,
	} {
		if got := yamlName(name); got != want {
			t.Errorf("yamlName(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestSkillDescriptionAppendsUseWhenFromFirstTriggers(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "Retrieval Evaluator",
		Description: "Compare retrieval runs and explain regressions",
		Triggers: []string{
			"You need a consistent workflow instead of one-off prompts.",
			"You want reproducible output that can be validated before shipping.",
			"You are comparing retrieval runs.",
		},
	}
	want := "Compare retrieval runs and explain regressions. Use when you need a consistent workflow instead of one-off prompts or you want reproducible output that can be validated before shipping."
	if got := skillDescription(spec); got != want {
		t.Fatalf("skillDescription() =\n%q\nwant\n%q", got, want)
	}
}

func TestSkillDescriptionKeepsExistingUseWhen(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "X",
		Description: "Lint configs. Use when the user asks to validate a config file.",
		Triggers:    []string{"You need something else."},
	}
	if got := skillDescription(spec); got != spec.Description {
		t.Fatalf("skillDescription() = %q, want unchanged %q", got, spec.Description)
	}
}

func TestSkillDescriptionWithoutTriggers(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{Name: "X", Description: "Summarize\n  papers   and notes."}
	if got, want := skillDescription(spec), "Summarize papers and notes."; got != want {
		t.Fatalf("skillDescription() = %q, want %q", got, want)
	}
}

func TestSkillDescriptionLengthLimits(t *testing.T) {
	t.Parallel()

	// Second trigger does not fit: fall back to the first only.
	spec := SkillSpec{
		Name:        "X",
		Description: strings.Repeat("a", 150),
		Triggers: []string{
			"You need " + strings.Repeat("b", 60),
			"You also need " + strings.Repeat("c", 100),
		},
	}
	got := skillDescription(spec)
	if utf8.RuneCountInString(got) > maxDescriptionLen {
		t.Fatalf("description is %d runes, want <= %d", utf8.RuneCountInString(got), maxDescriptionLen)
	}
	if !strings.Contains(got, "Use when you need bbbb") || strings.Contains(got, "ccc") {
		t.Fatalf("expected only the first trigger, got %q", got)
	}

	// No trigger fits: description is kept without a Use when clause.
	spec.Triggers = []string{"You need " + strings.Repeat("b", 400)}
	if got := skillDescription(spec); got != spec.Description {
		t.Fatalf("description with oversized trigger = %q, want unchanged", got)
	}

	// An over-long description is cut at a word boundary within the limit,
	// far below the 1024-character format limit.
	spec = SkillSpec{Name: "X", Description: strings.Repeat("lorem ipsum ", 200)}
	got = skillDescription(spec)
	if n := utf8.RuneCountInString(got); n > maxDescriptionLen || n < maxDescriptionLen-20 {
		t.Fatalf("truncated description is %d runes, want just under %d", n, maxDescriptionLen)
	}
	if !strings.HasSuffix(got, "...") || strings.HasSuffix(got, " ...") {
		t.Fatalf("truncated description should end with a clean ellipsis: %q", got)
	}

	// Multi-byte text is counted in runes and never cut mid-character.
	spec = SkillSpec{Name: "X", Description: strings.Repeat("检索评估 ", 200)}
	got = skillDescription(spec)
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) > maxDescriptionLen {
		t.Fatalf("multi-byte description invalid or too long: %d runes", utf8.RuneCountInString(got))
	}
}

func TestFrontmatterQuotesYAMLSpecialCharacters(t *testing.T) {
	t.Parallel()

	descriptions := []string{
		`Check: colon inside the text`,
		`Say "hello" and 'bye' and a \ backslash`,
		`* starts with an asterisk`,
		`& anchor-looking start`,
		`! tag-looking start`,
		`@ at sign and ` + "`backticks`",
		`# not a comment`,
		`[flow] {sequence} | pipe > fold`,
		`- dash start`,
		`key: value # trailing comment`,
		`true`,
		`123`,
		`Tab	and unicode — 检索 and <angle> & amp`,
	}
	for _, desc := range descriptions {
		spec := SkillSpec{Name: "Quote Test", Description: desc}
		doc := renderFrontmatter(spec) + "# Quote Test\n"
		fields := parseFrontmatter(t, doc)
		if fields["description"] != collapseSpace(desc) {
			t.Errorf("description round trip: got %q, want %q", fields["description"], collapseSpace(desc))
		}
		if strings.Count(strings.SplitN(doc, "\n---\n", 2)[0], "\n") != 2 {
			t.Errorf("frontmatter for %q is not exactly two lines:\n%s", desc, doc)
		}
	}

	quoted := yamlQuote("a \"b\" c\\d\ne\x01")
	if quoted != `"a \"b\" c\\d\ne\u0001"` {
		t.Errorf("yamlQuote escaping = %s", quoted)
	}
}

func TestScaffoldWritesLoadableSkill(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "Retrieval Evaluator: v2",
		Description: `Compare "retrieval" runs: find regressions`,
		Triggers:    []string{"You are comparing retrieval runs."},
		Tools:       []ToolSpec{{Name: "t", Command: "t --x", Description: "d"}},
	}
	outDir := filepath.Join(t.TempDir(), "retrieval-evaluator-v2")
	if err := scaffold(spec, outDir, false); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(outDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	fields := parseFrontmatter(t, string(body))
	if fields["name"] != "retrieval-evaluator-v2" || !validSkillName.MatchString(fields["name"]) {
		t.Errorf("name = %q", fields["name"])
	}
	if fields["name"] != filepath.Base(outDir) {
		t.Errorf("name %q does not match folder %q", fields["name"], filepath.Base(outDir))
	}
	desc := fields["description"]
	if desc == "" || utf8.RuneCountInString(desc) > maxDescriptionLen {
		t.Errorf("description %q has bad length", desc)
	}
	if !strings.Contains(desc, "Use when you are comparing retrieval runs.") {
		t.Errorf("description is missing the Use when clause: %q", desc)
	}
	if !strings.Contains(string(body), "\n# Retrieval Evaluator: v2\n") {
		t.Errorf("body heading missing or changed:\n%s", firstLines(string(body), 12))
	}

	manifest, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written SkillSpec
	if err := json.Unmarshal(manifest, &written); err != nil {
		t.Fatal(err)
	}
	if written.Slug != "retrieval-evaluator-v2" {
		t.Errorf("manifest slug = %q, want the sanitized skill name", written.Slug)
	}
}

func TestScaffoldWritesOpenAIYaml(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        `Research "Ops": notes`,
		Slug:        "researchops",
		Description: "Analyze local paper notes, compare experiment logs, and summarize retrieval findings for the whole team. Second sentence.",
		Tools:       []ToolSpec{{Name: "t", Command: "t", Description: "d"}},
	}
	outDir := filepath.Join(t.TempDir(), "researchops")
	if err := scaffold(spec, outDir, false); err != nil {
		t.Fatalf("scaffold() error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(outDir, "agents", "openai.yaml"))
	if err != nil {
		t.Fatalf("agents/openai.yaml missing: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 4 || lines[0] != "interface:" {
		t.Fatalf("unexpected openai.yaml shape:\n%s", body)
	}
	values := map[string]string{}
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("line %q is not indented under interface", line)
		}
		key, value, _ := strings.Cut(strings.TrimSpace(line), ": ")
		var decoded string
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			t.Fatalf("%s value %q is not a quoted scalar: %v", key, value, err)
		}
		values[key] = decoded
	}

	if values["display_name"] != spec.Name {
		t.Errorf("display_name = %q, want %q", values["display_name"], spec.Name)
	}
	short := values["short_description"]
	if short == "" || utf8.RuneCountInString(short) > maxShortDescriptionLen {
		t.Errorf("short_description %q has bad length %d", short, utf8.RuneCountInString(short))
	}
	if !strings.HasPrefix(short, "Analyze local paper notes") || strings.Contains(short, "Second sentence") {
		t.Errorf("short_description should shorten the first sentence, got %q", short)
	}
	prompt := values["default_prompt"]
	if !strings.HasPrefix(prompt, "Use $researchops to analyze local paper notes") || !strings.HasSuffix(prompt, ".") {
		t.Errorf("default_prompt = %q", prompt)
	}
}

func TestOpenAIYamlShortDescriptionNotTruncatedWhenShort(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{Name: "N", Slug: "n", Description: "Lint configs."}
	if got := shortDescription(spec); got != "Lint configs." {
		t.Errorf("shortDescription = %q", got)
	}
	if got := defaultPrompt(spec, "n"); got != "Use $n to lint configs." {
		t.Errorf("defaultPrompt = %q", got)
	}
}

func TestFolderNameWarning(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{Name: "Retrieval Evaluator", Slug: "retrieval-evaluator"}
	if got := folderNameWarning(spec, "/tmp/x/retrieval-evaluator"); got != "" {
		t.Errorf("unexpected warning for matching folder: %q", got)
	}
	if got := folderNameWarning(spec, "/tmp/x/retrieval-evaluator/"); got != "" {
		t.Errorf("unexpected warning for trailing slash: %q", got)
	}
	got := folderNameWarning(spec, "/tmp/x/out")
	if !strings.HasPrefix(got, "warning:") || strings.Contains(got, "\n") ||
		!strings.Contains(got, `"out"`) || !strings.Contains(got, `"retrieval-evaluator"`) {
		t.Errorf("unexpected warning: %q", got)
	}
}
