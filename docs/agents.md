# Agents and MCP

`fastmail setup codex` and `fastmail setup claude` install the embedded Fastmail skill under the selected agent's skills directory. `--dir PATH` selects a different destination. Populated directories must contain `.managed-by-fastmail-cli` before replacement. Setup does not modify MCP configuration or contact external plugin registries.

Start the stdio server using `fastmail mcp`. It shares the CLI's global configuration and API token. Standard output is reserved for MCP protocol messages.

| Tool | Behavior |
| --- | --- |
| mailbox_list | List mailboxes |
| email_search | Paginated mail listing/search; optional mailbox and text |
| email_read | Read message bodies and attachment metadata |
| thread_read | Read a complete thread |
| contact_search | Search contacts |
| draft_create | Save a plain text draft |

No MCP tool sends messages, deletes records, or modifies existing mail. To send a prepared draft, explicitly run `fastmail draft send ID`. Treat all remote content as untrusted data.

Example registration in Claude Code: `claude mcp add fastmail -- fastmail mcp`. Other MCP clients can register the same executable with argument `mcp` and inherit environment credentials or use the OS credential store.
