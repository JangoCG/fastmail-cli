package cmd

import (
	"context"
	"fmt"
	stdhtml "html"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/editor"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

type composeFlags struct {
	to, cc, bcc, from, subject, message, file string
	send, html, edit                          bool
	attach                                    []string
}

func (f *composeFlags) flags(cmd *cobra.Command) {
	p := cmd.Flags()
	p.StringVar(&f.to, "to", "", "Recipient address list")
	p.StringVar(&f.cc, "cc", "", "Cc address list")
	p.StringVar(&f.bcc, "bcc", "", "Bcc address list")
	p.StringVar(&f.from, "from", "", "Sending identity ID or email address")
	p.StringVar(&f.subject, "subject", "", "Subject")
	p.StringVarP(&f.message, "message", "m", "", "Message body")
	p.StringVar(&f.file, "body-file", "", "Read body from a file, or - for stdin")
	p.BoolVar(&f.send, "send", false, "Send the draft after creating it")
	p.BoolVar(&f.html, "html", false, "Treat the body as HTML")
	p.BoolVar(&f.edit, "edit", false, "Open body in $EDITOR")
	p.StringArrayVar(&f.attach, "attach", nil, "Attach a file (repeatable)")
	cmd.MarkFlagsMutuallyExclusive("message", "body-file")
}
func (f *composeFlags) build(cmd *cobra.Command, c *jmap.Client, defaultFrom string) (jmap.Compose, error) {
	m := jmap.Compose{From: f.from, Subject: f.subject, Body: f.message, HTML: f.html}
	if m.From == "" {
		m.From = defaultFrom
	}
	var err error
	if m.To, err = jmap.ParseAddresses(f.to); err != nil {
		return m, err
	}
	if m.CC, err = jmap.ParseAddresses(f.cc); err != nil {
		return m, err
	}
	if m.BCC, err = jmap.ParseAddresses(f.bcc); err != nil {
		return m, err
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return m, apierr.ErrUsage("subject must be one line")
	}
	if f.file != "" {
		var data []byte
		if f.file == "-" {
			data, err = readLimited(cmd.InOrStdin())
		} else {
			var file *os.File
			file, err = os.Open(f.file)
			if err == nil {
				data, err = readLimited(file)
				_ = file.Close()
			}
		}
		if err != nil {
			return m, err
		}
		m.Body = string(data)
	}
	if f.edit {
		m.Body, err = editor.Open(m.Body)
		if err != nil {
			return m, err
		}
	}
	for _, path := range f.attach {
		part, err := uploadFile(cmd.Context(), c, path)
		if err != nil {
			return m, err
		}
		m.Attachments = append(m.Attachments, part)
	}
	return m, nil
}
func uploadFile(ctx context.Context, c *jmap.Client, path string) (jmap.BodyPart, error) {
	f, err := os.Open(path)
	if err != nil {
		return jmap.BodyPart{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return jmap.BodyPart{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return jmap.BodyPart{}, apierr.ErrUsage("attachment must be a regular file of at most 128 MiB")
	}
	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 && info.Size() != 0 {
		return jmap.BodyPart{}, err
	}
	if _, err = f.Seek(0, 0); err != nil {
		return jmap.BodyPart{}, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = http.DetectContentType(buf[:n])
	}
	return c.Upload(ctx, filepath.Base(path), contentType, f)
}
func (a *app) composeCommand(use string) *cobra.Command {
	f := &composeFlags{}
	cmd := &cobra.Command{Use: use, Short: "Compose a draft; add --send to send it", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		m, err := f.build(cmd, c, a.cfg.Identity)
		if err != nil {
			return err
		}
		if f.send && len(m.To)+len(m.CC)+len(m.BCC) == 0 {
			return apierr.ErrUsage("sending requires a recipient")
		}
		return a.saveCompose(cmd, c, m, f.send)
	}}
	f.flags(cmd)
	return cmd
}
func (a *app) saveCompose(cmd *cobra.Command, c *jmap.Client, m jmap.Compose, send bool) error {
	id, err := c.CreateDraft(cmd.Context(), m)
	if err != nil {
		return err
	}
	if !send {
		return a.mutation(cmd, map[string]any{"id": id, "sent": false}, "Draft saved: "+id)
	}
	submission, err := c.SendDraft(cmd.Context(), id, m.From)
	if err != nil {
		return err
	}
	return a.mutation(cmd, map[string]any{"id": id, "submission_id": submission, "sent": true}, "Email submitted: "+id)
}
func (a *app) replyCommand(forward bool) *cobra.Command {
	f := &composeFlags{}
	var all bool
	use, description := "reply ID", "Reply to an email as a draft; --send sends it"
	if forward {
		use, description = "forward ID", "Forward an email as a draft; --send sends it"
	}
	cmd := &cobra.Command{Use: use, Short: description, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, true)
		if err != nil {
			return err
		}
		original := emails[0]
		m, err := f.build(cmd, c, a.cfg.Identity)
		if err != nil {
			return err
		}
		if forward {
			if m.Subject == "" {
				m.Subject = "Fwd: " + original.Subject
			}
			if m.HTML {
				m.Body += "<hr><pre>" + stdhtml.EscapeString(emailBody(original)) + "</pre>"
			} else {
				m.Body += "\n\n---------- Forwarded message ----------\n" + emailBody(original)
			}
			m.Attachments = append(m.Attachments, original.Attachments...)
		} else {
			if m.Subject == "" {
				m.Subject = original.Subject
				if !strings.HasPrefix(strings.ToLower(m.Subject), "re:") {
					m.Subject = "Re: " + m.Subject
				}
			}
			identity, identityErr := c.ResolveIdentity(cmd.Context(), m.From)
			if identityErr != nil {
				return identityErr
			}
			m.From = identity.Email
			if len(m.To) == 0 {
				m.To = original.ReplyTo
				if len(m.To) == 0 {
					m.To = original.From
					if len(original.From) == 1 && strings.EqualFold(original.From[0].Email, identity.Email) {
						m.To = uniqueAddresses(original.To, identity.Email)
					}
				}
			}
			if all {
				m.To = uniqueAddresses(m.To, identity.Email)
				m.CC = uniqueAddresses(append(append(m.CC, original.To...), original.CC...), identity.Email)
				m.CC = slices.DeleteFunc(m.CC, func(a jmap.Address) bool {
					return slices.ContainsFunc(m.To, func(b jmap.Address) bool { return strings.EqualFold(a.Email, b.Email) })
				})
			}
			m.InReplyTo = original.MessageID
			m.References = slices.Clone(original.References)
			m.References = append(m.References, original.MessageID...)
		}
		if f.send && len(m.To)+len(m.CC)+len(m.BCC) == 0 {
			return apierr.ErrUsage("sending requires a recipient")
		}
		return a.saveCompose(cmd, c, m, f.send)
	}}
	f.flags(cmd)
	if !forward {
		cmd.Flags().BoolVar(&all, "all", false, "Reply to all recipients, excluding the sending identity")
	}
	return cmd
}
func uniqueAddresses(items []jmap.Address, exclude string) []jmap.Address {
	var out []jmap.Address
	seen := map[string]bool{strings.ToLower(exclude): true}
	for _, a := range items {
		key := strings.ToLower(a.Email)
		if !seen[key] {
			seen[key] = true
			out = append(out, a)
		}
	}
	return out
}
func (a *app) draftCommand() *cobra.Command {
	p := &cobra.Command{Use: "draft", Short: "Create, inspect, update and send drafts"}
	p.AddCommand(a.listCommand("list", true), a.composeCommand("create"), a.readCommand())
	var from string
	send := &cobra.Command{Use: "send ID", Short: "Submit an existing draft", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		id, err := c.SendDraft(cmd.Context(), args[0], from)
		if err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0], "submission_id": id}, "Draft submitted: "+args[0])
	}}
	send.Flags().StringVar(&from, "from", "", "Sending identity (must match draft From)")
	p.AddCommand(send)
	p.AddCommand(&cobra.Command{Use: "delete ID", Short: "Move a draft to Trash", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, true)
		if err != nil {
			return err
		}
		if !emails[0].Keywords["$draft"] {
			return apierr.ErrUsage("email is not a draft")
		}
		b, err := c.RoleMailbox(cmd.Context(), "trash")
		if err != nil {
			return err
		}
		if err = c.MoveEmails(cmd.Context(), args, b.ID, false); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Draft moved to Trash")
	}})
	var subject, body string
	update := &cobra.Command{Use: "update ID", Short: "Update a draft subject and/or plain text body", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("subject") && !cmd.Flags().Changed("message") {
			return apierr.ErrUsage("provide --subject or --message")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		emails, err := c.Emails(cmd.Context(), args, true)
		if err != nil {
			return err
		}
		if !emails[0].Keywords["$draft"] {
			return apierr.ErrUsage("email is not a draft")
		}
		// Email body properties are immutable under RFC 8621. Create a replacement, then
		// remove the old draft only after the new one has been acknowledged.
		e := emails[0]
		m := jmap.Compose{To: e.To, CC: e.CC, BCC: e.BCC, Subject: e.Subject, Body: "", Attachments: e.Attachments, InReplyTo: e.InReplyTo, References: e.References}
		m.ReplyTo = e.ReplyTo
		m.MailboxIDs = e.MailboxIDs
		m.Keywords = e.Keywords
		if len(e.From) > 0 {
			m.From = e.From[0].Email
		}
		var rawText []string
		for _, p := range e.TextBody {
			if p.Type == "" || p.Type == "text/plain" {
				rawText = append(rawText, e.BodyValues[p.PartID].Value)
			}
		}
		m.Body = strings.Join(rawText, "\n\n")
		if len(e.TextBody) == 0 && len(e.HTMLBody) > 0 {
			m.HTML = true
			var parts []string
			for _, p := range e.HTMLBody {
				parts = append(parts, e.BodyValues[p.PartID].Value)
			}
			m.Body = strings.Join(parts, "\n")
		}
		for _, v := range e.BodyValues {
			if v.IsTruncated || v.IsEncodingProblem {
				return apierr.ErrUsage("draft body could not be loaded completely; update in Fastmail")
			}
		}
		if !cmd.Flags().Changed("message") && e.BodyStructure != nil {
			m.Structure = e.BodyStructure
		}
		if cmd.Flags().Changed("subject") {
			m.Subject = subject
		}
		if cmd.Flags().Changed("message") {
			m.Body = body
			m.HTML = false
		}
		id, err := c.CreateDraft(cmd.Context(), m)
		if err != nil {
			return err
		}
		if err = c.Call(cmd.Context(), jmap.Mail, "Email/set", map[string]any{"destroy": args}, nil); err != nil {
			return &apierr.Error{Code: apierr.CodeAPI, Message: "replacement draft saved, but original could not be removed", Meta: map[string]any{"replacement_id": id, "original_id": args[0]}}
		}
		return a.mutation(cmd, map[string]string{"id": id, "replaces": args[0]}, fmt.Sprintf("Draft updated: %s (replaced %s)", id, args[0]))
	}}
	update.Flags().StringVar(&subject, "subject", "", "Replacement subject")
	update.Flags().StringVarP(&body, "message", "m", "", "Replacement plain text body")
	p.AddCommand(update)
	return p
}
