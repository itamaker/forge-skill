package app

import (
	"bytes"
	"encoding/json"
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

	fallbackSkillName = "generated-skill"
)

// skillName returns the Agent Skills `name` for a spec: lowercase ASCII
// letters, digits and single hyphens, no leading or trailing hyphen, at most
// 64 characters. It prefers the spec slug and falls back to the display name.
func skillName(spec SkillSpec) string {
	if name := sanitizeSkillName(spec.Slug); name != "" {
		return name
	}
	if name := sanitizeSkillName(spec.Name); name != "" {
		return name
	}
	return fallbackSkillName
}

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
	desc := collapseSpace(spec.Description)
	if desc == "" {
		desc = collapseSpace(spec.Name)
	}

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
				return candidate
			}
		}
	}
	return truncateWords(desc, maxDescriptionLen)
}

func hasUseWhen(text string) bool {
	return strings.Contains(strings.ToLower(text), "use when")
}

// triggerClause turns a trigger such as "You need a repeatable review." into
// the tail of a "Use when ..." clause: "you need a repeatable review".
func triggerClause(trigger string) string {
	trigger = strings.TrimRight(collapseSpace(trigger), ".;:!, ")
	if trigger == "" {
		return ""
	}
	first, size := utf8.DecodeRuneInString(trigger)
	rest := trigger[size:]
	// Keep acronyms and identifiers ("RAG runs", "SKILL.md") intact.
	if unicode.IsUpper(first) {
		next, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsUpper(next) && !unicode.IsDigit(next) {
			trigger = string(unicode.ToLower(first)) + rest
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
func yamlQuote(text string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(text); err != nil {
		return `""`
	}
	return strings.TrimSuffix(buf.String(), "\n")
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
