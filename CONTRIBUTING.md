# Contributing

Use Go 1.27.3. Read AGENTS.md and STYLE.md, then run `make build`, `make test` and `make lint`.

Add behavior tests beside implementation files. JMAP tests should exercise actual request/response JSON against httptest servers, including per-object failures and partial success. CalDAV tests should verify discovery, iCalendar preservation and conditional writes. Never use live Fastmail credentials in tests.

Keep documentation and command help aligned. The module is github.com/JangoCG/fastmail-cli. Use local builds to verify changes before publishing. Publishing binaries, installing agent skills, changing remotes and sending real mail require an explicit operator request.

The original MIT copyright remains in LICENSE.md.
