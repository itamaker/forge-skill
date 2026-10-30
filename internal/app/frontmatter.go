package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// maxSkillNameLen is the Agent Skills limit for the `name` field.
	maxSkillNameLen = 64
	// maxDescriptionLen is the length skillforge targets for the model-facing
	// `description` (the Agent Skills hard limit is 1024; shorter descriptions
	// keep every installed skill cheap to list in the agent's context).
	maxDescriptionLen = 300
	// maxShortDescriptionLen is the length of interface.short_description in
	// agents/openai.yaml.
	maxShortDescriptionLen = 64
	// maxDefaultPromptTaskLen bounds the task text inside default_prompt.
	maxDefaultPromptTaskLen = 160
)

// skillName returns the Agent Skills `name` for a spec: lowercase ASCII
// letters, digits and single hyphens, no leading or trailing hyphen, at most
// 64 characters. It prefers the spec slug and falls back to the display name.
// It returns "" when neither yields a usable name (for example a name written
// only in CJK characters); callers must treat that as an error.
func skillName(spec SkillSpec) string {
	if name := sanitizeSkillName(spec.Slug); name != "" {
		return name
	}
	return sanitizeSkillName(spec.Name)
}

// errNoSkillName is returned when a spec has no usable ASCII skill name.
var errNoSkillName = errors.New("spec has no usable ASCII skill name; set an ASCII `slug` in the spec")

func sanitizeSkillName(raw string) string {
	var builder strings.Builder
	prevDash := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			builder.WriteByte('-')
			prevDash = true
		}
	}
	name := builder.String()
	if len(name) > maxSkillNameLen {
		name = name[:maxSkillNameLen]
	}
	return strings.Trim(name, "-")
}

// skillDescription builds the single-line, model-facing description: the spec
// description with whitespace collapsed and, when the description has no
// "Use when" clause and the spec has triggers, a concise clause built from the
// first one or two triggers. The result never exceeds maxDescriptionLen.
func skillDescription(spec SkillSpec) string {
	desc, _ := buildDescription(spec)
	return desc
}

// buildDescription is skillDescription plus human-readable notes about content
// that had to be dropped to stay within maxDescriptionLen.
func buildDescription(spec SkillSpec) (string, []string) {
	desc := collapseSpace(spec.Description)
	if desc == "" {
		desc = collapseSpace(spec.Name)
	}

	var notes []string
	if !hasUseWhen(desc) {
		var triggers []string
		for _, trigger := range spec.Triggers {
			if clause := triggerClause(trigger); clause != "" {
				triggers = append(triggers, clause)
			}
			if len(triggers) == 2 {
				break
			}
		}
		base := ensureSentenceEnd(desc)
		for n := len(triggers); n > 0; n-- {
			candidate := base + " Use when " + strings.Join(triggers[:n], " or ") + "."
			if utf8.RuneCountInString(candidate) <= maxDescriptionLen {
				return candidate, nil
			}
		}
		if len(triggers) > 0 {
			notes = append(notes, "note: the description has no room for a \"Use when\" clause within the 300-character limit; shorten the description or add the clause yourself")
		}
	}
	if utf8.RuneCountInString(desc) > maxDescriptionLen {
		notes = append(notes, "note: the description was cut to the 300-character limit")
	}
	return truncateWords(desc, maxDescriptionLen), notes
}

var useWhenPattern = regexp.MustCompile(`(?i)\buse when\b`)

// hasUseWhen reports whether text already contains a "use when" clause as a
// whole phrase. Negated uses ("do not use when ...") do not count.
func hasUseWhen(text string) bool {
	for _, loc := range useWhenPattern.FindAllStringIndex(text, -1) {
		before := strings.ToLower(strings.TrimRight(text[:loc[0]], " "))
		if strings.HasSuffix(before, "not") || strings.HasSuffix(before, "never") || strings.HasSuffix(before, "n't") {
			continue
		}
		return true
	}
	return false
}

// triggerClause turns a trigger such as "You need a repeatable review." into
// the tail of a "Use when ..." clause: "you need a repeatable review". Only a
// plain capitalised first word is lowercased; "GitHub", "OAuth", "Q&A",
// "SKILL.md" and the pronoun "I" are left alone.
func triggerClause(trigger string) string {
	trigger = strings.TrimRight(collapseSpace(trigger), ".;:!, ")
	if trigger == "" {
		return ""
	}
	token := trigger
	if i := strings.IndexByte(trigger, ' '); i >= 0 {
		token = trigger[:i]
	}
	first, size := utf8.DecodeRuneInString(token)
	if unicode.IsUpper(first) && token != "I" && !strings.HasPrefix(token, "I'") && !strings.HasPrefix(token, "I\u2019") {
		plain := true
		for _, r := range token[size:] {
			if unicode.IsUpper(r) {
				plain = false
				break
			}
		}
		if plain {
			trigger = string(unicode.ToLower(first)) + trigger[size:]
		}
	}
	return trigger
}

func ensureSentenceEnd(text string) string {
	if text == "" {
		return text
	}
	switch text[len(text)-1] {
	case '.', '!', '?':
		return text
	}
	return text + "."
}

// shortDescription is the first sentence of the description (without any
// appended "Use when" clause), cut to fit interface.short_description.
func shortDescription(spec SkillSpec) string {
	return truncateWords(firstSentence(spec), maxShortDescriptionLen)
}

func firstSentence(spec SkillSpec) string {
	desc := collapseSpace(spec.Description)
	if desc == "" {
		desc = collapseSpace(spec.Name)
	}
	for i := 0; i < len(desc)-1; i++ {
		if (desc[i] == '.' || desc[i] == '!' || desc[i] == '?') && desc[i+1] == ' ' {
			return desc[:i+1]
		}
	}
	return desc
}

// defaultPrompt is the suggested starter prompt; Codex requires it to mention
// the skill as $<name>.
func defaultPrompt(spec SkillSpec, name string) string {
	task := strings.TrimRight(firstSentence(spec), ".!? ")
	task = truncateWords(task, maxDefaultPromptTaskLen)
	task = strings.TrimRight(task, ".!? ")
	if task == "" {
		return "Use $" + name + "."
	}
	return "Use $" + name + " to " + triggerClause(task) + "."
}

// truncateWords shortens text to at most limit runes, cutting at a word
// boundary and marking the cut with "...".
func truncateWords(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := runes[:limit-3]
	if i := lastSpace(cut); i > limit/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(string(cut), " ,;:-.") + "..."
}

func lastSpace(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}
	return -1
}

func collapseSpace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// yamlQuote renders text as a double-quoted YAML scalar. A JSON string is a
// valid YAML double-quoted scalar, so this escapes quotes, backslashes,
// newlines and control characters and is safe for values starting with
// characters such as '*', '&', '!', '@', '#', '[' or containing ': ' or ' #'.
// JSON leaves some characters raw that a YAML stream may not contain (DEL, C1
// controls, U+FFFE/U+FFFF) or that YAML parsers treat as line breaks (U+0085);
// those are written as \uXXXX escapes.
func yamlQuote(text string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(text); err != nil {
		return `""`
	}
	encoded := strings.TrimSuffix(buf.String(), "\n")

	var out strings.Builder
	for _, r := range encoded {
		if yamlSafeRune(r) {
			out.WriteRune(r)
			continue
		}
		fmt.Fprintf(&out, `\u%04X`, r)
	}
	return out.String()
}

// yamlSafeRune reports whether r may appear raw in a YAML double-quoted
// scalar: the YAML c-printable set minus NEL and the byte order mark.
func yamlSafeRune(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return true
	case r >= 0x20 && r <= 0x7E:
		return true
	case r >= 0xA0 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return r != 0xFEFF
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	}
	return false
}

// yamlName renders the skill name. It is unquoted unless YAML would read it
// as a non-string (a number, date, boolean or null).
func yamlName(name string) string {
	switch name {
	case "true", "false", "yes", "no", "on", "off", "null", "y", "n":
		return yamlQuote(name)
	}
	if name != "" && name[0] >= '0' && name[0] <= '9' {
		return yamlQuote(name)
	}
	return name
}

func renderFrontmatter(spec SkillSpec) string {
	name := skillName(spec)
	return "---\n" +
		"name: " + yamlName(name) + "\n" +
		"description: " + yamlQuote(skillDescription(spec)) + "\n" +
		"---\n\n"
}

func renderOpenAIYaml(spec SkillSpec) string {
	name := skillName(spec)
	display := collapseSpace(spec.Name)
	if display == "" {
		display = name
	}
	return "interface:\n" +
		"  display_name: " + yamlQuote(display) + "\n" +
		"  short_description: " + yamlQuote(shortDescription(spec)) + "\n" +
		"  default_prompt: " + yamlQuote(defaultPrompt(spec, name)) + "\n"
}
