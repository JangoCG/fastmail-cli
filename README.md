# Fastmail CLI

[Source](https://github.com/JangoCG/fastmail-cli) · [CLI reference](docs/cli.md)

A Go command-line client for Fastmail mail, contacts and calendars, with an interactive mailbox reader and an MCP server. This is an independent project, derived from the MIT-licensed HEY CLI; it is not affiliated with Fastmail or 37signals.

## Build and sign in

```sh
go install github.com/JangoCG/fastmail-cli/cmd/fastmail@latest
fastmail auth login
fastmail tui
```

Or build from source:

```sh
git clone https://github.com/JangoCG/fastmail-cli.git
cd fastmail-cli
make build
./bin/fastmail auth login
./bin/fastmail mailbox list
./bin/fastmail tui
```

Requires Go 1.27.3. `make install` installs `fastmail` under `/usr/local/bin`; override `PREFIX` to choose another location. Install from Go source or build the repository locally. Prebuilt release packages and automatic updates are not configured.

Create a **JMAP API token** in [Fastmail settings](https://app.fastmail.com/settings/security/tokens). Enable Email, Email submission, Contacts, Masked Email and Vacation Response where available for the features you need. The CLI checks the capabilities actually advertised by your token. A read-only token can be used for reads. OAuth is not configured: Fastmail requires registration of each OAuth client.

Login prompts for a hidden token. For automation, set `FASTMAIL_TOKEN`, or pipe a token from your secret manager into `fastmail auth login --token-stdin`. Tokens use the OS keyring, with a private file fallback. `FASTMAIL_NO_KEYRING=1` forces file storage.

## Examples

```sh
fastmail email list --mailbox inbox --unread
fastmail search --from alice@example.com --after 2026-10-01
fastmail email read EMAIL_ID --markdown
fastmail thread read THREAD_ID
fastmail compose --to alice@example.com --subject 'Lunch on Friday' -m 'Does noon work?'
fastmail draft send DRAFT_ID
fastmail reply EMAIL_ID --all -m 'Friday works for me.'
fastmail attachment download EMAIL_ID BLOB_ID --output ./invoice.pdf
fastmail email archive EMAIL_ID
fastmail contact search 'Alice'
fastmail masked-email list
fastmail watch
```

`compose`, `reply` and `forward` save drafts by default. Add `--send` to submit them, or explicitly run `draft send`. Sending is never automatically retried after an uncertain response.

Output is readable at a terminal and JSON when piped. `--json`, `--quiet`, `--ids-only`, `--count`, `--markdown` and `--jq` support scripting:

```sh
fastmail email list --mailbox inbox --jq '.data[] | {id, subject}'
fastmail email list --mailbox inbox --all --ids-only
```

## Calendars

Fastmail exposes calendars through CalDAV. Store a separate **app password** with calendar access (the account password and JMAP API token do not work here):

```sh
# Pipe your app password from a secret manager into:
fastmail auth caldav --username alice@example.com
fastmail calendar list
fastmail event list --calendar 'Personal' --after 2026-10-01 --before 2026-11-01
fastmail event add 'Design review' --calendar 'Personal' \
  --start 2026-10-30T14:00:00+01:00 --end 2026-10-30T15:00:00+01:00
```

## Coding agents

```sh
fastmail setup codex
fastmail setup claude
claude mcp add fastmail -- fastmail mcp
```

The MCP server reads mail and contacts and creates drafts. It has no sending or deletion tools. Setup writes only the Fastmail skill, and refuses to replace an unmanaged skill directory.

## Documentation and development

- [CLI reference](docs/cli.md)
- [Authentication and configuration](docs/auth.md)
- [Interactive reader](docs/tui.md)
- [Agents and MCP](docs/agents.md)
- [Migration and protocol decisions](docs/migration.md)

```sh
make build
make test
make lint
```

Tests use isolated HTTP JMAP and CalDAV servers and temporary credential stores. No account credentials or real mail server are needed. See [CONTRIBUTING.md](CONTRIBUTING.md).

MIT licensed; original copyright retained in [LICENSE.md](LICENSE.md).
