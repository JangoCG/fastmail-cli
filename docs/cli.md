# CLI reference

Run `fastmail COMMAND --help` for the exact flags. The generated [command inventory](commands.txt) lists every command. JMAP IDs and cursors are opaque strings; use IDs returned by list/search operations.

## Output and exit status

Terminal output is human-readable; redirected output defaults to an envelope: `{"ok":true,"data":...}`. Mutations add a summary. Errors are written to stderr with `ok:false`, `code`, `error`, and optional `hint` / `meta`.

| Flag | Effect |
| --- | --- |
| --json | JSON envelope |
| --quiet | JSON data without the envelope |
| --ids-only | One ID per line, for list results |
| --count | Number of returned items, not the server's total |
| --markdown | Markdown table; email/thread reads print body Markdown |
| --styled | Human-readable output even when piped |
| --jq EXPR | jq expression over the envelope; with --quiet over data |
| --account ID | Select JMAP account |
| --session-url URL | Override JMAP session discovery |

Output-format flags are mutually exclusive. Errors exit 1 for usage, validation or conflict; 2 for missing resources; 3 for authentication; 4 for permission; 5 for rate limit; 6 for network failure; 7 for API failure; 8 for ambiguity. `watch` always writes NDJSON; `mcp` reserves stdout for its transport. Completion and help print plain text.

## Authentication and preferences

- `auth login [--token-stdin]`: validate and save token. Uses a hidden terminal prompt or FASTMAIL_TOKEN otherwise.
- `auth status`: validate the current token and show accounts.
- `auth logout`: remove stored API and CalDAV credentials; does not revoke remote tokens or unset environment credentials.
- `auth caldav --username EMAIL`: validate and save an app password from stdin.
- `accounts`: list accessible accounts and capabilities.
- `identity list`: list sending identities; set `config set identity EMAIL` to choose the default when there are multiple identities.
- `config show`, `config set KEY VALUE`: manage account, identity, session-url, dav-url and dav-username.
- `doctor`: verify JMAP access and token capabilities.
- `version`: show build metadata.
- `completion bash|zsh|fish|powershell`: generate shell completion.

See [authentication](auth.md) for storage and environment variables.

## Mailboxes, listing and search

`mailbox` (alias `folder`) has `list`, `create NAME [--parent MAILBOX]`, `rename MAILBOX NAME`, and `delete MAILBOX`. Deletion is restricted to empty, non-system mailboxes. A mailbox selector accepts an ID, an unambiguous name, or a system role such as inbox, archive, drafts, sent or trash. Exact IDs take precedence; role-specific internal operations require the actual role.

`email` (alias `mail`) has `list [TEXT...]`, and `search [TEXT...]` is also available at the top level. Filter with `--mailbox`, `--from`, `--to`, `--subject`, `--after`, `--before`, `--unread`, `--flagged`, or `--has-attachment`. Dates accept YYYY-MM-DD (UTC midnight) or RFC3339 with timezone; --before is exclusive. Search text is passed to JMAP's standard text filter, not Fastmail's web search query syntax.

Listings return newest-first messages with `--limit` (default 50, maximum 1000) and `--position` (default 0). `--all` fetches successive pages. JSON meta contains total and next_position. Pagination is positional; concurrent mailbox changes can shift results across pages.

## Reading and changing mail

- `email read ID`: metadata, loaded body values, attachment metadata. `--markdown` renders text/HTML as readable Markdown. JSON preserves per-part truncation and encoding flags. Reading does not mark the email seen.
- `thread read ID`: load messages in a JMAP thread, including bodies.
- `email raw ID --output FILE`: download the original .eml to a new file.
- `email seen|unseen ID...`: set or remove $seen.
- `email star|unstar ID...`: set or remove $flagged.
- `email archive ID...`: remove Inbox membership and add the archive mailbox, preserving other labels.
- `email trash ID...`: move exclusively to Trash.
- `email move ID... --mailbox MAILBOX`: replace mailbox memberships with the destination.
- `email copy ID... --mailbox MAILBOX`: add mailbox membership, preserving existing memberships; the JMAP message itself is not duplicated.
- `email delete ID... --yes`: permanently destroy messages.

JMAP per-object failures are errors even when the HTTP response succeeds. A batch may partially succeed; error meta includes the set response to inspect what changed. Mutations are never automatically retried.

## Composing and drafts

`compose` and `draft create` accept `--to`, `--cc`, `--bcc`, `--from`, `--subject`, `--message` / `-m`, `--body-file FILE` (or `-` for stdin), `--html`, `--edit` ($EDITOR), and repeatable `--attach FILE`. --message and --body-file are mutually exclusive. --from accepts an identity ID or email; if multiple identities exist, configure a default or supply one.

The default is to **save a draft**. `--send` saves and then submits it. A sending command requires at least one recipient. `reply ID [--all]` pre-fills Reply-To or From, preserves threading references, excludes the selected sending identity from reply-all recipients, and replies to the original recipients when continuing a message sent by that identity. `forward ID --to EMAIL` includes the source as quoted plain text and preserves its attachments. Both save drafts unless --send is supplied.

`draft list` accepts the same filters and pagination as email list. `draft read ID` reads an email; it does not enforce draft status. `draft send ID [--from EMAIL]` verifies the email is still a draft and that the chosen identity matches its From field. Successful submission removes $draft and moves Drafts membership to Sent; unrelated memberships survive.

`draft update ID [--subject TEXT] [-m TEXT]` creates a replacement because JMAP email content is immutable. It preserves recipients, threading and attachments, then destroys the old draft after the replacement is acknowledged. The returned ID changes. If removal fails, the error reports both IDs. `draft delete ID` moves a verified draft to Trash.

A lost submission response can occur after a message has been sent. Inspect Sent and the error's draft ID before retrying.

## Attachments

`attachment list EMAIL_ID` lists blobs attached to a message. `attachment download EMAIL_ID BLOB_ID [--output FILE_OR_DIR] [--force]` verifies the blob belongs to that message, sanitizes derived filenames and saves through an atomic file write. Existing files are preserved unless --force is supplied. Downloads are never opened or executed.

`attachment upload FILE` uploads a blob and returns its metadata; it does not attach it to an existing email. Use compose --attach for that. Uploads and downloads are capped at 128 MiB; Fastmail may enforce a smaller account limit.

## Contacts

`addressbook list` shows address books and access rights. `contact` (alias `contacts`) supports:

- `list [TEXT...]`, `search TEXT...`: --limit, --position and --all paginate matching JSContact cards.
- `read ID`: complete JSContact object.
- `add --name NAME [--email EMAIL] [--phone NUMBER] [--addressbook ID]`: create in a writable default or explicitly selected book.
- `update ID [--name NAME] [--email EMAIL] [--phone NUMBER]`: change the full name or the CLI-owned email / phone field. Other addresses and metadata are preserved. Empty --email or --phone removes only that CLI field.
- `delete ID --yes`: permanently delete a card.

## Masked Email and vacation

`masked-email list` lists addresses. `masked-email create [--for-domain ORIGIN] [--description TEXT] [--prefix PREFIX]` creates an enabled address; --for-domain is an origin such as https://example.com. Prefix characters and rate limits are checked by Fastmail. `masked-email update ID [--state enabled|disabled|deleted] [--description TEXT]` changes an address. Disabled sends incoming mail to Trash; deleted bounces incoming mail.

`vacation get` reads the singleton reply. `vacation set -m TEXT [--subject TEXT] [--from DATE] [--until DATE]` enables a reply and replaces the prior message and date range. Omitted dates mean no bound. `vacation disable` turns it off while preserving the message.

## Calendars and events

These commands use separate CalDAV app-password credentials, with no JMAP token required.

`calendar list` returns calendar paths and supported components. Select calendars by path or unambiguous name. If there is only one, it is the default.

`event list [--calendar SELECTOR] [--after DATE] [--before DATE]` lists resources overlapping a period (default now through one month later). Recurrences are not expanded into occurrences. `event read PATH` includes complete ICS and ETag. PATH is an .ics resource returned by listing.

`event add TITLE --calendar SELECTOR --start TIME --end TIME` creates an event. Times use RFC3339 with timezone. Add `--all-day` to use YYYY-MM-DD; the end date is exclusive. Optional --description and --location set text fields.

`event update PATH [--title TEXT] [--description TEXT] [--location TEXT] [--start TIME --end TIME]` updates a nonrecurring event while preserving all other ICS fields. Time changes require both start and end; use --all-day for date values. Concurrent updates fail with a conflict instead of overwriting server changes.

`event delete PATH --yes` deletes an event resource. A recurring resource additionally requires --series and deletes the entire series. Individual recurrence edits and invitation scheduling belong in Fastmail.

## Changes and agents

`watch [--since STATE] [--interval 15s] [--once]` emits account-wide NDJSON records: ready, created, updated, destroyed, checkpoint. It observes email objects, including drafts and sent mail. Without --since it starts at the current state and does not replay existing mail. Persist checkpoint.state to resume. --once drains all pending changes. Delivery is at least once across restarts; clients should deduplicate IDs and checkpoints. An expired cursor is an error. Polling exits on rate limits or transport failures and does not silently reset the baseline.

`setup codex|claude [--dir PATH]` installs the Fastmail agent skill. `mcp` serves reading and draft creation through stdio, without sending or deletion tools. See [agents](agents.md). `tui` opens the [interactive mailbox reader](tui.md).
