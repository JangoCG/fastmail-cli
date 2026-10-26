# Go style

Write direct Go that is easy to follow. Keep related operations together, use concrete types for protocol data, and add abstractions only when they serve multiple real callers.

## Organization

`cmd/fastmail` owns process startup, cancellation and exit status. `internal/cmd` owns Cobra commands and output formatting. Keep one command family per file. The app object holds configuration and clients for a single invocation; avoid mutable package globals. Command structs are useful for shared flag groups; simple commands can use local flag variables and closures.

Handlers validate arguments before making network calls, obtain the authenticated JMAP client with `a.api(cmd.Context())`, and pass that context to all requests. CalDAV handlers obtain `a.calendarAPI()` independently; a calendar-only invocation does not need a JMAP token. Cobra constructs command handlers before executing them; use cmd.Context() when a handler actually runs.

All JMAP methods go through `internal/jmap`. Keep transport, protocol decoding and per-object error handling there. Protocol IDs and state strings are never converted to numbers. Use typed structs for stable responses and explicit maps for JMAP PatchObject fields and extensible JSContact objects. Escape JSON pointer segments with jmap.PatchPath.

All calendar reads and writes go through `internal/dav`. Preserve unknown iCalendar fields and reject unguarded writes. Do not add a generic raw HTTP CLI that bypasses feature-specific validation.

## Output and errors

Use app.result for data and app.mutation for mutation results. Structured output goes through internal/output, with machine-readable apierr errors and useful recovery IDs for partial success. Errors must not include credentials or raw server error bodies.

Preserve original content in protocol models. Sanitize remote strings only where they reach a terminal: terminal.Sanitize for bodies, SanitizeLine for a single row. HTML is converted through htmlutil.ToMarkdown; never render arbitrary email markup as terminal control sequences. JSON uses output.MarshalJSON to escape control characters.

## Functions and tests

Put exported entry points before their helpers, and keep helpers near their callers. Prefer explicit branches over clever control flow. Close files and HTTP bodies, check errors that affect the result, and avoid shadowing an outer error variable.

Test observable behavior with httptest servers and temporary storage. Include negative paths for irreversible actions, protocol-level partial failures and concurrent calendar changes. Test the actual CLI binary in tests/smoke, and use MCP's in-memory transport for tool-level checks. Do not use real credentials or send real mail during tests.

Run make build, make test and make lint before declaring a change ready. make lint includes go vet, gofmt and the pinned golangci-lint checks. Update command help, docs/cli.md and the command inventory whenever the public interface changes.
