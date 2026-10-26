# Fastmail CLI

This repository is an independent Fastmail CLI, originally derived from the MIT-licensed HEY CLI. The active binary is `bin/fastmail`, built from `cmd/fastmail`. The module and repository are `github.com/JangoCG/fastmail-cli`.

## Development

- Read STYLE.md before writing Go.
- `make build` builds the binary used for manual checks.
- `make test` runs all tests with the race detector.
- `make lint` runs go vet, pinned golangci-lint and checks gofmt.
- Keep history linear; rebase instead of merging main.
- Never use account credentials or a real server for automated tests. Use httptest and isolated config directories.
- Use example.com or example.org addresses in examples.

## Architecture

- `internal/jmap`: Fastmail session discovery, capability/account selection, RFC 8620 transport, mail, contacts and change cursors. All JMAP requests go through this package; never scrape pages.
- `internal/dav`: Fastmail calendar access through CalDAV and iCalendar. The DAV app password is separate from the JMAP API token. Updates/deletes must use ETags; creates use If-None-Match.
- `internal/auth`, `internal/config`: endpoint-scoped keyring credentials, private JSON fallback files, atomic configuration writes. Never load project-local credentials/config automatically.
- `internal/cmd`: Cobra commands. Data-access commands initialize the authenticated client before any requests. Human mutation messages and JSON envelopes go through app.mutation.
- `internal/tui`: Bubble Tea v2 mailbox reader. Network operations run as commands; remote content is sanitized at display time.
- `internal/mcpserver`: MCP read tools and draft creation. No sending or deletion tools.
- `internal/output`, `internal/terminal`, `internal/htmlutil`, `internal/markdown`, `internal/attachments`: tested output, terminal safety, HTML-to-Markdown conversion and safe file writes retained from the original project.

## Invariants

JMAP IDs and states are opaque strings. Check /set per-object errors, not just HTTP or method success. Mutations must never be automatically retried, especially EmailSubmission/set. Email content is immutable: changing a draft body creates a replacement before deleting the old draft. Report partial failures and IDs so users can recover.

Nothing from remote metadata or message bodies reaches terminal sinks without terminal.Sanitize / SanitizeLine. JSON uses output.MarshalJSON so control codes remain escaped. Do not interpolate raw email HTML into terminal output.

Compose/reply/forward default to draft creation; --send or draft send is explicit. MCP never sends. Agent skill writes must check .managed-by-fastmail-cli before replacing a populated directory.

Update command help and docs/cli.md for command changes. Regenerate docs/commands.txt using the command inventory test's -update-commands option when the command surface changes. Explain protocol limitations honestly in docs/migration.md.
