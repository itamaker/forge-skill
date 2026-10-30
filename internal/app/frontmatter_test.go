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
		// The 64th character is a hyphen: it must be trimmed, not kept.
		{strings.Repeat("a", 63) + "-b", strings.Repeat("a", 63)},
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

	if got := skillName(SkillSpec{Name: "检索评估", Slug: "检索"}); got != "" {
		t.Errorf("skillName with unusable name = %q, want empty (no silent fallback)", got)
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

	// No trigger fits: description is kept without a Use when clause, and
	// the caller is told so.
	spec.Triggers = []string{"You need " + strings.Repeat("b", 400)}
	got, notes := buildDescription(spec)
	if got != spec.Description {
		t.Fatalf("description with oversized trigger = %q, want unchanged", got)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Use when") {
		t.Fatalf("notes = %q, want one note about the omitted Use when clause", notes)
	}
	if _, notes := buildDescription(SkillSpec{Name: "X", Description: "Short."}); len(notes) != 0 {
		t.Fatalf("no triggers, no note expected, got %q", notes)
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

func TestSanitizeSkillNameIsASCIIOnly(t *testing.T) {
	t.Parallel()

	if got := slugify("检索评估"); got != "" {
		t.Errorf("slugify(CJK) = %q, want empty", got)
	}
	if got := slugify("Café Tool 检索"); got != "caf-tool" {
		t.Errorf("slugify mixed = %q, want caf-tool", got)
	}
	// draft must show the slug init will use.
	spec := draftSpec("# Retrieval Evaluator\nx\n", nil, "", 1)
	if spec.Slug != skillName(spec) {
		t.Errorf("draft slug %q != init skill name %q", spec.Slug, skillName(spec))
	}
	cjk := draftSpec("# 检索评估\n检索评估技能\n", nil, "", 1)
	if cjk.Slug != "" || skillName(cjk) != "" {
		t.Errorf("CJK draft slug = %q, name = %q, want both empty", cjk.Slug, skillName(cjk))
	}
}

func TestScaffoldRejectsSpecWithoutASCIIName(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "检索评估",
		Description: "x",
		Tools:       []ToolSpec{{Name: "t", Command: "t", Description: "d"}},
	}
	outDir := filepath.Join(t.TempDir(), "out")
	// An existing output dir must survive a rejected spec even with force.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := scaffold(spec, outDir, true)
	if err == nil || !strings.Contains(err.Error(), "set an ASCII `slug` in the spec") {
		t.Fatalf("scaffold() error = %v, want the ASCII slug message", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("rejected spec must not touch the output dir: %v", statErr)
	}

	spec.Slug = "retrieval-evaluator"
	if err := scaffold(spec, filepath.Join(t.TempDir(), "retrieval-evaluator"), false); err != nil {
		t.Errorf("an ASCII slug should make the spec valid: %v", err)
	}
}

func TestYAMLQuoteEscapesCharactersInvalidInYAML(t *testing.T) {
	t.Parallel()

	got := yamlQuote("a\x7fb\u0080c\u0085d\u009fe\ufffe\uffff\ufeff\u2028f")
	want := `"a\u007Fb\u0080c\u0085d\u009Fe\uFFFE\uFFFF\uFEFF\u2028f"`
	if got != want {
		t.Fatalf("yamlQuote =\n%s\nwant\n%s", got, want)
	}

	// Property: nothing in the output is outside the YAML c-printable set,
	// whatever the input.
	inputs := []string{"plain", "tab\tnew\nline", "检索 — ok", "\U0001F600 emoji", "bad\xffutf8", "\x00\x01\x1f\x7f"}
	for r := rune(0); r < 0x300; r++ {
		inputs = append(inputs, "x"+string(r)+"y")
	}
	for _, in := range inputs {
		out := yamlQuote(in)
		for _, r := range out {
			if r == '\n' || r == '\r' || !yamlSafeRune(r) {
				t.Errorf("yamlQuote(%q) = %s contains unsafe rune %U", in, out, r)
				break
			}
		}
		// Still a valid JSON string, i.e. a valid YAML double-quoted scalar.
		var back string
		if err := json.Unmarshal([]byte(out), &back); err != nil {
			t.Errorf("yamlQuote(%q) = %s is not a valid quoted scalar: %v", in, out, err)
		}
	}
}

func TestTriggerClause(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"You need a repeatable review.": "you need a repeatable review",
		"GitHub issues pile up":         "GitHub issues pile up",
		"OAuth tokens expire":           "OAuth tokens expire",
		"JavaScript bundles grow":       "JavaScript bundles grow",
		"Q&A pages need review":         "Q&A pages need review",
		"RAG runs regress":              "RAG runs regress",
		"SKILL.md files are missing":    "SKILL.md files are missing",
		"I need a second opinion.":      "I need a second opinion",
		"I'm reviewing a diff":          "I'm reviewing a diff",
		"Reviewing a diff; again":       "reviewing a diff; again",
		"already lowercase":             "already lowercase",
		"  Spaces   collapse. ":         "spaces collapse",
		"":                              "",
		"3 files changed":               "3 files changed",
		"Écrire des notes":              "écrire des notes",
	}
	for in, want := range cases {
		if got := triggerClause(in); got != want {
			t.Errorf("triggerClause(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultPromptKeepsProperNouns(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"GitHub issue triage and labelling.": "Use $x to GitHub issue triage and labelling.",
		"Triage GitHub issues.":              "Use $x to triage GitHub issues.",
		"I review diffs.":                    "Use $x to I review diffs.",
		"":                                   "Use $x to name.",
	}
	for desc, want := range cases {
		if got := defaultPrompt(SkillSpec{Name: "name", Description: desc}, "x"); got != want {
			t.Errorf("defaultPrompt(%q) = %q, want %q", desc, got, want)
		}
	}
}

func TestHasUseWhenWholePhraseOnly(t *testing.T) {
	t.Parallel()

	yes := []string{"Lint configs. Use when asked.", "use when the user asks", "USE WHEN needed", "x. Use when, in doubt, ask"}
	no := []string{
		"A reuse when possible policy",
		"Prevents misuse whenever it can",
		"Use whenever you like",
		"Do not use when offline",
		"Never use when offline",
		"Don't use when offline",
		"Lint configs.",
	}
	for _, text := range yes {
		if !hasUseWhen(text) {
			t.Errorf("hasUseWhen(%q) = false, want true", text)
		}
	}
	for _, text := range no {
		if hasUseWhen(text) {
			t.Errorf("hasUseWhen(%q) = true, want false", text)
		}
	}

	// A misleading substring no longer suppresses the generated clause.
	spec := SkillSpec{Name: "X", Description: "Encourages reuse whenever possible.", Triggers: []string{"You lint configs."}}
	if got := skillDescription(spec); !strings.HasSuffix(got, "Use when you lint configs.") {
		t.Errorf("skillDescription() = %q, want a Use when clause appended", got)
	}
}

func TestScaffoldRewritesInvalidSlugInManifest(t *testing.T) {
	t.Parallel()

	spec := SkillSpec{
		Name:        "Research Ops",
		Slug:        "Bad Slug!!",
		Description: "x",
		Tools:       []ToolSpec{{Name: "t", Command: "t", Description: "d"}},
	}
	outDir := filepath.Join(t.TempDir(), "bad-slug")
	if err := scaffold(spec, outDir, false); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written SkillSpec
	if err := json.Unmarshal(manifest, &written); err != nil {
		t.Fatal(err)
	}
	if written.Slug != "bad-slug" {
		t.Errorf("manifest slug = %q, want bad-slug", written.Slug)
	}
	skill, err := os.ReadFile(filepath.Join(outDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if fields := parseFrontmatter(t, string(skill)); fields["name"] != "bad-slug" {
		t.Errorf("frontmatter name = %q, want bad-slug", fields["name"])
	}
	if !strings.Contains(string(skill), "- Slug: `bad-slug`") {
		t.Errorf("Metadata slug line was not rewritten:\n%s", firstLines(string(skill), 14))
	}
}
