# Interactive mailbox reader

Run `fastmail tui` after signing in. It opens the Inbox, lists 50 emails per page and refreshes the visible mailbox every 30 seconds while the list is open. Reading does not automatically mark a message seen.

| Key | Action |
| --- | --- |
| Up / Down, k / j | Select message |
| Enter | Read message and attachment metadata |
| Escape / Backspace | Return to list |
| Tab / Shift+Tab | Next / previous mailbox |
| n / p | Next / previous page |
| u | Toggle read/unread |
| s | Toggle star |
| e | Archive selected message |
| t | Move selected message to Trash |
| r | Refresh |
| q / Ctrl+C | Quit |

The message reader scrolls with arrows and Page Up / Page Down. Long text wraps. The CLI provides composition, contact editing, calendar operations and attachment downloads; use each command's --help. Network errors appear in the status line and allow refreshing or quitting.
