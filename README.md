# skillforge

> **Moved.** The Claude Code skill for this tool now lives in [itamaker/skills](https://github.com/itamaker/skills/tree/main/skills/agent-tooling/skillforge), together with my other skills. Install from there: `npx skills@latest add itamaker/skills --skill=skillforge`.
>
> This repository still hosts the command-line tool's source and releases.

[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)

`skillforge` is a Go CLI for drafting and scaffolding OpenClaw-ready skills from briefs, tool catalogs, and JSON specs.

It helps agent teams standardize skill packaging and bootstrap new skills without building a separate internal generator service.

![skillforge social preview](docs/images/social-preview.png)

## Support

[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-FFDD00?style=for-the-badge&logo=buy-me-a-coffee&logoColor=black)](https://buymeacoffee.com/amaker)

## Quickstart

### Install

```bash
brew install itamaker/tap/skillforge
```

<details>
<summary>You can also download binaries from <a href="https://github.com/itamaker/skillforge/releases">GitHub Releases</a>.</summary>

Current release archives:

- macOS (Apple Silicon/arm64): `skillforge_0.2.0_darwin_arm64.tar.gz`
- macOS (Intel/x86_64): `skillforge_0.2.0_darwin_amd64.tar.gz`
- Linux (arm64): `skillforge_0.2.0_linux_arm64.tar.gz`
- Linux (x86_64): `skillforge_0.2.0_linux_amd64.tar.gz`

Each archive contains a single executable: `skillforge`.

</details>

### First Run

Run:

```bash
skillforge
```

This launches the interactive Bubble Tea terminal UI.

You can still use the direct command form:

```bash
skillforge init -spec examples/skill.json -out /tmp/research-skill
```

Or draft a richer spec from a short brief plus a tool catalog:

```bash
skillforge draft -brief examples/brief.md -catalog examples/tools.json -out /tmp/research-skill.json
```

The generated directory includes:

- `SKILL.md`, starting with YAML frontmatter (`name`, `description`) so Claude Code, Codex and skills.sh can load it
- `agents/openai.yaml` (`interface.display_name`, `short_description`, `default_prompt`)
- `manifest.json`
- `bin/README.md`
- `examples/usage.md`

### Frontmatter and folder name

- `name` is the spec `slug` (or the slugified `name`), sanitised to lowercase letters, digits and single hyphens, at most 64 characters.
- `description` is the spec description as one YAML-quoted line, at most 300 characters. If it has no "Use when" clause and the spec has `triggers`, a `Use when ...` clause built from the first one or two triggers is appended.
- Claude Code and Codex expect the skill folder name to equal `name`. Name the `-out` folder after the slug; `init` prints a one-line warning on stderr (exit code unchanged) when it differs.

### Review drafted specs before `init`

`draft` fills `triggers`, `constraints`, `checks` and `workflow` from category defaults. It prints a `review:` note on stderr to say so (stdout and the spec JSON are unchanged). Edit those fields, especially `triggers`, which feed the SKILL.md description, before running `init`.

### Help and version

```bash
skillforge --help          # also: -h, help
skillforge init --help     # also: skillforge help init
skillforge draft --help
skillforge --version       # also: version
```

Help and version print to stdout and exit `0`. Usage errors exit `2`, failures exit `1`.

Running `skillforge` with no arguments (or `skillforge tui`) launches the interactive terminal UI. Scripts and coding agents must not do that; use `init` and `draft`.

## Requirements

- Go `1.22+`

## Run

```bash
go run .
```

Or run the non-interactive command directly:

```bash
go run . init -spec examples/skill.json -out /tmp/research-skill
```

Use `-force` if you want to overwrite an existing output directory.

Generate a spec first:

```bash
go run . draft -brief examples/brief.md -catalog examples/tools.json
```

## Build From Source

```bash
make build
```

```bash
go build -o dist/skillforge .
```

## What It Does

1. Loads a compact JSON skill spec.
2. Validates required fields such as `name`, `description`, and `tools`.
3. Can draft a richer spec from a natural-language brief by retrieving relevant tools from a local catalog.
4. Generates a portable skill folder with a frontmatter-headed `SKILL.md`, `agents/openai.yaml`, workflow, constraints, checks, docs, and a manifest.
5. Produces output that can be dropped into a Claude Code, Codex, or OpenClaw-style skills folder.

## Notes

- Use `examples/skill.json` as a starting point for new skill definitions.
- Maintainer release steps live in `PUBLISHING.md`.

## Claude Code skill

The Claude Code skill for this tool lives in [itamaker/skills](https://github.com/itamaker/skills/tree/main/skills/agent-tooling/skillforge). Install it from there:

```text
/plugin marketplace add itamaker/skills
/plugin install itamaker-skills@itamaker
```

Or install just this skill with `npx skills@latest add itamaker/skills --skill=skillforge`. The skill drives this command-line tool, so install the tool first.

## Contributors ✨

| [![Zhaoyang Jia][avatar-zhaoyang]][author-zhaoyang] |
| --- |
| [Zhaoyang Jia][author-zhaoyang] |



[author-zhaoyang]: https://github.com/itamaker
[avatar-zhaoyang]: https://images.weserv.nl/?url=https://github.com/itamaker.png&h=120&w=120&fit=cover&mask=circle&maxage=7d

## License

[MIT](LICENSE)
