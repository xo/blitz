# Contributing to blitz

Read three documents before you change anything.

[`AGENTS.md`](AGENTS.md) holds the rules: the standing rules, the layout, the
commands and the Go conventions. It is written for a coding agent, and
everything in it applies to a person too. `CLAUDE.md` holds one line that
imports it, for Claude Code.

[`docs/PLAN.md`](docs/PLAN.md) holds the plan and every decision, with the
reason for each. The table at the top lists each decision with its status.
Read the status, because some decisions amend an earlier one.

[`README.md`](README.md) explains how to use the package, how to rebuild the
archives, and the limits of each platform.

Do not decide an open question on your own. The open questions are at the end
of `docs/PLAN.md`. Ask Ken.

## Before you send a change

Run these commands in the repository root. Each one must pass, and
`gofmt -l .` must print nothing:

```sh
gofmt -l .
go vet ./...
go test -race ./...
make check-generated
```

A render needs fonts. On Linux, install `fontconfig` and one font family, such
as `fonts-dejavu-core`, before you run the tests.

If you rebuild the archives, rebuild all six targets in one commit, with
`version.txt`. The README says how, under "Rebuilding the archives".

## Agent skills

The repository carries two agent skills. A skill is a set of instructions
that a coding agent loads for a task. `simple-english` sets how prose is
written, and `go-pedantry` sets how Go is written.

`skills-lock.json` names the source of each skill. The `skills` command from
npm writes that file, and version 1.7.0 is the one measured. It writes each
skill into two folders. Codex and the other agents read
`.agents/skills/<name>`, and Claude Code reads `.claude/skills/<name>`.

To install or update a skill, run its command in the repository root:

```sh
npx skills@1.7.0 add AminBlg/SimpleEnglish --skill simple-english --agent codex claude-code --copy -y
npx skills@1.7.0 add oborchers/fractional-cto --skill go-pedantry --agent codex claude-code --copy -y
```

Keep `--copy`. Without it, the command writes `.claude/skills/<name>` as a
symbolic link. A Windows checkout writes a symbolic link as a small text file,
and Claude Code then loads no skill and reports nothing. `TestSkillsAreCopies`
fails on a link, on a missing copy, and on two copies that differ. See D22.

`.claude/settings.local.json` holds the Claude Code permissions of one
person. The root `.gitignore` ignores it.
