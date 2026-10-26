# Migration from HEY CLI

The HTTP integration, authentication, command model, TUI and MCP have been rewritten for Fastmail. This independent repository is published at https://github.com/JangoCG/fastmail-cli with its own Git history. The original project is https://github.com/basecamp/hey-cli; its MIT copyright and license are retained. The executable is now `fastmail`; there is no `hey` compatibility executable. Existing HEY credentials, configuration and installed agent skills are not read or modified.

Mail uses JMAP Core and Mail (RFC 8620 / RFC 8621); sending uses EmailSubmission. Contacts use JMAP Contacts (RFC 9610 / JSContact). Calendars use CalDAV (RFC 4791) because Fastmail has not opened JMAP calendar access. Masked Email uses Fastmail's advertised extension; vacation replies use the JMAP vacationresponse capability. All URLs come from session discovery or explicit configuration.

HEY-specific Screener, Imbox, Feed, Paper Trail, bundles, collections, clips, journal, habits and time tracking have no equivalent Fastmail API model and were removed. Fastmail mailboxes and labels are exposed as mailboxes. The HEY Action Cable subscription has been replaced with polling Email/changes. The TUI is a mailbox reader with pagination, read/unread, stars, archive and trash. Contacts and calendar editing are available through the CLI. Compose and reply are CLI draft operations.

There is no shared OAuth registration, browser-cookie login, published package, self-updater, Omarchy integration or upstream publisher in this fork. Use a personal API token and install from Go source or build locally. Releases can be packaged with GoReleaser; uploading is disabled until an actual destination is configured.

Calendar listing returns resources matching the requested period, not expanded recurring occurrences. Event reads retain the full ICS. Creation supports timed and all-day events. Updates preserve unrelated ICS fields and refuse recurring/multi-event resources. Deleting a recurring resource requires both --yes and --series and deletes the entire resource. Individual occurrence editing and invitation scheduling should be done in Fastmail.

Body values are capped at 4 MiB per part; JSON retains isTruncated / isEncodingProblem. Read the raw .eml or use Fastmail for larger or malformed bodies. Draft replacement refuses incomplete bodies. Attachments are limited to 128 MiB by the client, with additional account limits enforced by Fastmail. A submission response failure can follow successful sending: inspect Sent and the returned draft ID before retrying.

Protocol references: [Fastmail developer documentation](https://www.fastmail.com/dev/), [JMAP Mail](https://jmap.io/spec/rfc8621/), [JMAP Contacts](https://www.rfc-editor.org/rfc/rfc9610.html), [CalDAV](https://www.rfc-editor.org/rfc/rfc4791.html).
