package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/attachments"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/output"
	"github.com/JangoCG/fastmail-cli/internal/terminal"
)

func (a *app) mailboxCommand() *cobra.Command {
	p := &cobra.Command{Use: "mailbox", Aliases: []string{"folder"}, Short: "List and manage Fastmail folders and labels"}
	p.AddCommand(&cobra.Command{Use: "list", Short: "List mailboxes and unread counts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		boxes, err := c.Mailboxes(cmd.Context())
		if err != nil {
			return err
		}
		var lines []string
		for _, b := range boxes {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%d unread / %d\t%s", b.ID, b.Name, b.UnreadEmails, b.TotalEmails, b.Role))
		}
		return a.result(cmd, boxes, strings.Join(lines, "\n"))
	}})
	var parent string
	create := &cobra.Command{Use: "create NAME", Short: "Create a mailbox", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		obj := map[string]any{"name": args[0]}
		if parent != "" {
			b, parentErr := c.ResolveMailbox(cmd.Context(), parent)
			if parentErr != nil {
				return parentErr
			}
			obj["parentId"] = b.ID
		}
		var r jmap.SetResult
		if err = c.Call(cmd.Context(), jmap.Mail, "Mailbox/set", map[string]any{"create": map[string]any{"mailbox": obj}}, &r); err != nil {
			return err
		}
		return a.mutation(cmd, r, "Mailbox created")
	}}
	create.Flags().StringVar(&parent, "parent", "", "Parent mailbox ID or name")
	p.AddCommand(create)
	p.AddCommand(&cobra.Command{Use: "rename MAILBOX NAME", Short: "Rename a mailbox", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		b, err := c.ResolveMailbox(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if err = c.Call(cmd.Context(), jmap.Mail, "Mailbox/set", map[string]any{"update": map[string]any{b.ID: map[string]any{"name": args[1]}}}, nil); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": b.ID, "name": args[1]}, "Mailbox renamed")
	}})
	p.AddCommand(&cobra.Command{Use: "delete MAILBOX", Short: "Delete an empty, non-system mailbox", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		b, err := c.ResolveMailbox(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if b.Role != "" || b.TotalEmails > 0 {
			return apierr.ErrUsage("only empty, non-system mailboxes can be deleted")
		}
		if err = c.Call(cmd.Context(), jmap.Mail, "Mailbox/set", map[string]any{"destroy": []string{b.ID}, "onDestroyRemoveEmails": false}, nil); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": b.ID}, "Mailbox deleted")
	}})
	return p
}

type emailListCommand struct {
	cmd                                   *cobra.Command
	box, from, to, subject, after, before string
	unread, flagged, hasAttachment, all   bool
	limit, position                       int
	drafts                                bool
}

func (a *app) listCommand(use string, drafts bool) *cobra.Command {
	l := &emailListCommand{drafts: drafts}
	l.cmd = &cobra.Command{Use: use, Short: "List or search email (newest first)", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if l.limit < 1 || l.limit > 1000 || l.position < 0 {
			return apierr.ErrUsage("--limit must be 1..1000 and --position must be nonnegative")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		filter := map[string]any{}
		if l.drafts {
			b, err := c.RoleMailbox(cmd.Context(), "drafts")
			if err != nil {
				return err
			}
			filter["inMailbox"] = b.ID
		} else if l.box != "" {
			b, err := c.ResolveMailbox(cmd.Context(), l.box)
			if err != nil {
				return err
			}
			filter["inMailbox"] = b.ID
		}
		for key, value := range map[string]string{"from": l.from, "to": l.to, "subject": l.subject} {
			if value != "" {
				filter[key] = value
			}
		}
		text := strings.TrimSpace(strings.Join(args, " "))
		if text != "" {
			filter["text"] = text
		}
		for key, value := range map[string]string{"after": l.after, "before": l.before} {
			if value != "" {
				date, err := jmap.Date(value)
				if err != nil {
					return err
				}
				filter[key] = date
			}
		}
		if l.unread {
			filter["notKeyword"] = "$seen"
		}
		if l.flagged {
			filter["hasKeyword"] = "$flagged"
		}
		if l.hasAttachment {
			filter["hasAttachment"] = true
		}
		emails := []jmap.Email{}
		position := l.position
		var total int
		for {
			page, q, err := c.ListEmails(cmd.Context(), filter, position, l.limit)
			if err != nil {
				return err
			}
			emails = append(emails, page...)
			total = q.Total
			position += len(page)
			if !l.all || len(page) == 0 || position >= q.Total {
				break
			}
		}
		return a.result(cmd, emails, emailLines(emails), output.WithMeta("total", total), output.WithMeta("next_position", position))
	}}
	f := l.cmd.Flags()
	f.StringVar(&l.box, "mailbox", "", "Mailbox ID, name or role (default: all mail)")
	f.StringVar(&l.from, "from", "", "Filter sender")
	f.StringVar(&l.to, "to", "", "Filter recipient")
	f.StringVar(&l.subject, "subject", "", "Filter subject")
	f.StringVar(&l.after, "after", "", "Received on/after date or RFC3339 time")
	f.StringVar(&l.before, "before", "", "Received before date or RFC3339 time")
	f.BoolVar(&l.unread, "unread", false, "Only unread messages")
	f.BoolVar(&l.flagged, "flagged", false, "Only starred messages")
	f.BoolVar(&l.hasAttachment, "has-attachment", false, "Only messages with attachments")
	f.IntVar(&l.limit, "limit", 50, "Page size (1..1000)")
	f.IntVar(&l.position, "position", 0, "Zero-based start position")
	f.BoolVar(&l.all, "all", false, "Fetch every matching page")
	return l.cmd
}
func emailLines(emails []jmap.Email) string {
	lines := make([]string, 1, len(emails)+1)
	lines[0] = "ID\tDATE\tFROM\tSUBJECT"
	for _, e := range emails {
		sender := ""
		if len(e.From) > 0 {
			sender = e.From[0].Email
		}
		mark := " "
		if !e.Keywords["$seen"] {
			mark = "*"
		}
		lines = append(lines, fmt.Sprintf("%s%s\t%s\t%s\t%s", mark, e.ID, e.ReceivedAt, sender, e.Subject))
	}
	return strings.Join(lines, "\n")
}
func emailBody(e jmap.Email) string {
	return fmt.Sprintf("%s\nFrom: %s\nTo: %s\nDate: %s\nID: %s\n\n%s", e.Subject, jmap.FormatAddresses(e.From), jmap.FormatAddresses(e.To), e.ReceivedAt, e.ID, e.Body())
}
func (a *app) readCommand() *cobra.Command {
	return &cobra.Command{Use: "read ID", Short: "Read an email, including bodies and attachment metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, true)
		if err != nil {
			return err
		}
		if a.markdown && a.jq == "" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), terminal.Sanitize(emailBody(emails[0])))
			return err
		}
		return a.result(cmd, emails[0], emailBody(emails[0]))
	}}
}
func (a *app) emailCommand() *cobra.Command {
	p := &cobra.Command{Use: "email", Aliases: []string{"mail"}, Short: "Read and manage email"}
	p.AddCommand(a.listCommand("list [TEXT...]", false), a.readCommand())
	var destination string
	raw := &cobra.Command{Use: "raw ID", Short: "Download original RFC 5322 message to a new file", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if destination == "" {
			return apierr.ErrUsage("--output is required")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, false)
		if err != nil {
			return err
		}
		data, err := c.Download(cmd.Context(), emails[0].BlobID, "message.eml", "message/rfc822")
		if err != nil {
			return err
		}
		n, err := attachments.SaveBytes(destination, data, false)
		if err != nil {
			return err
		}
		return a.mutation(cmd, map[string]any{"path": destination, "bytes": n}, "Message saved to "+destination)
	}}
	raw.Flags().StringVarP(&destination, "output", "o", "", "Destination file (required)")
	p.AddCommand(raw)
	for _, action := range []string{"seen", "unseen", "star", "unstar", "archive", "trash", "move", "copy", "delete"} {
		var box string
		var yes bool
		ccmd := &cobra.Command{Use: action + " ID...", Short: action + " email by ID", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			if action == "delete" && !yes {
				return apierr.ErrUsage("permanent deletion requires --yes")
			}
			c, err := a.api(cmd.Context())
			if err != nil {
				return err
			}
			switch action {
			case "seen":
				err = c.UpdateEmails(cmd.Context(), args, map[string]any{"keywords/$seen": true})
			case "unseen":
				err = c.UpdateEmails(cmd.Context(), args, map[string]any{"keywords/$seen": nil})
			case "star":
				err = c.UpdateEmails(cmd.Context(), args, map[string]any{"keywords/$flagged": true})
			case "unstar":
				err = c.UpdateEmails(cmd.Context(), args, map[string]any{"keywords/$flagged": nil})
			case "archive":
				err = c.Archive(cmd.Context(), args)
			case "trash":
				var b jmap.Mailbox
				b, err = c.RoleMailbox(cmd.Context(), "trash")
				if err == nil {
					err = c.MoveEmails(cmd.Context(), args, b.ID, false)
				}
			case "move", "copy":
				if box == "" {
					return apierr.ErrUsage("--mailbox is required")
				}
				err = c.MoveEmails(cmd.Context(), args, box, action == "copy")
			case "delete":
				err = c.Call(cmd.Context(), jmap.Mail, "Email/set", map[string]any{"destroy": args}, nil)
			}
			if err != nil {
				return err
			}
			return a.mutation(cmd, map[string]any{"ids": args, "action": action}, fmt.Sprintf("%s: %d email(s)", action, len(args)))
		}}
		if action == "move" || action == "copy" {
			ccmd.Flags().StringVar(&box, "mailbox", "", "Target mailbox ID, name or role")
		}
		if action == "delete" {
			ccmd.Flags().BoolVar(&yes, "yes", false, "Confirm permanent deletion")
		}
		p.AddCommand(ccmd)
	}
	return p
}
func (a *app) searchCommand() *cobra.Command { return a.listCommand("search [TEXT...]", false) }
func (a *app) threadCommand() *cobra.Command {
	p := &cobra.Command{Use: "thread", Short: "Read conversations"}
	p.AddCommand(&cobra.Command{Use: "read ID", Short: "Read every email in a JMAP thread", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Thread(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		var body []string
		for _, e := range emails {
			body = append(body, emailBody(e))
		}
		text := strings.Join(body, "\n\n---\n\n")
		if a.markdown && a.jq == "" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), terminal.Sanitize(text))
			return err
		}
		return a.result(cmd, emails, text)
	}})
	return p
}
func (a *app) attachmentCommand() *cobra.Command {
	p := &cobra.Command{Use: "attachment", Short: "List, upload and download attachments"}
	p.AddCommand(&cobra.Command{Use: "list EMAIL_ID", Short: "List attachments and blob IDs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, true)
		if err != nil {
			return err
		}
		var lines []string
		for _, part := range emails[0].Attachments {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%d bytes", part.BlobID, part.Name, part.Size))
		}
		return a.result(cmd, emails[0].Attachments, strings.Join(lines, "\n"))
	}})
	var out string
	var force bool
	download := &cobra.Command{Use: "download EMAIL_ID BLOB_ID", Short: "Save an email attachment", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args[:1], true)
		if err != nil {
			return err
		}
		for _, part := range emails[0].Attachments {
			if part.BlobID != args[1] {
				continue
			}
			dest, err := attachments.Destination(out, part.Name)
			if err != nil {
				return err
			}
			data, err := c.Download(cmd.Context(), part.BlobID, part.Name, part.Type)
			if err != nil {
				return err
			}
			n, err := attachments.SaveBytes(dest, data, force)
			if err != nil {
				return err
			}
			return a.mutation(cmd, map[string]any{"path": dest, "bytes": n}, "Attachment saved to "+dest)
		}
		return apierr.ErrNotFound("attachment", args[1])
	}}
	download.Flags().StringVarP(&out, "output", "o", "", "Output file or directory")
	download.Flags().BoolVar(&force, "force", false, "Replace existing destination")
	p.AddCommand(download)
	p.AddCommand(&cobra.Command{Use: "upload FILE", Short: "Upload a blob for use in JMAP requests", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		part, err := uploadFile(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}
		return a.mutation(cmd, part, "Attachment uploaded: "+part.BlobID)
	}})
	return p
}
