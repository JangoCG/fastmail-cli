package jmap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/htmlutil"
	"github.com/JangoCG/fastmail-cli/internal/terminal"
)

type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

func FormatAddresses(addresses []Address) string {
	parts := make([]string, 0, len(addresses))
	for _, a := range addresses {
		value := a.Email
		if a.Name != "" {
			value = a.Name + " <" + a.Email + ">"
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, ", ")
}

type BodyPart struct {
	PartID      string     `json:"partId,omitempty"`
	BlobID      string     `json:"blobId,omitempty"`
	Type        string     `json:"type"`
	Name        string     `json:"name,omitempty"`
	Size        int64      `json:"size,omitempty"`
	Disposition string     `json:"disposition,omitempty"`
	Charset     string     `json:"charset,omitempty"`
	CID         string     `json:"cid,omitempty"`
	Language    []string   `json:"language,omitempty"`
	Location    string     `json:"location,omitempty"`
	SubParts    []BodyPart `json:"subParts,omitempty"`
}
type BodyValue struct {
	Value             string `json:"value"`
	IsTruncated       bool   `json:"isTruncated"`
	IsEncodingProblem bool   `json:"isEncodingProblem"`
}
type Email struct {
	ID            string               `json:"id"`
	BlobID        string               `json:"blobId"`
	ThreadID      string               `json:"threadId"`
	MailboxIDs    map[string]bool      `json:"mailboxIds"`
	Keywords      map[string]bool      `json:"keywords"`
	MessageID     []string             `json:"messageId"`
	InReplyTo     []string             `json:"inReplyTo"`
	References    []string             `json:"references"`
	From          []Address            `json:"from"`
	ReplyTo       []Address            `json:"replyTo"`
	To            []Address            `json:"to"`
	CC            []Address            `json:"cc"`
	BCC           []Address            `json:"bcc,omitempty"`
	Subject       string               `json:"subject"`
	ReceivedAt    string               `json:"receivedAt"`
	Preview       string               `json:"preview"`
	HasAttachment bool                 `json:"hasAttachment"`
	TextBody      []BodyPart           `json:"textBody,omitempty"`
	HTMLBody      []BodyPart           `json:"htmlBody,omitempty"`
	Attachments   []BodyPart           `json:"attachments,omitempty"`
	BodyStructure *BodyPart            `json:"bodyStructure,omitempty"`
	BodyValues    map[string]BodyValue `json:"bodyValues,omitempty"`
}

func (e Email) Body() string {
	var parts []string
	for _, p := range e.TextBody {
		parts = append(parts, e.BodyValues[p.PartID].Value)
	}
	if len(parts) > 0 {
		return terminal.Sanitize(strings.Join(parts, "\n\n"))
	}
	for _, p := range e.HTMLBody {
		parts = append(parts, htmlutil.ToMarkdown(e.BodyValues[p.PartID].Value).String())
	}
	return strings.Join(parts, "\n\n")
}

type Mailbox struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Role         string  `json:"role"`
	ParentID     *string `json:"parentId"`
	TotalEmails  int     `json:"totalEmails"`
	UnreadEmails int     `json:"unreadEmails"`
	SortOrder    int     `json:"sortOrder"`
}
type Identity struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	MayDelete bool   `json:"mayDelete"`
}
type GetResult[T any] struct {
	List     []T      `json:"list"`
	NotFound []string `json:"notFound"`
	State    string   `json:"state"`
}
type QueryResult struct {
	IDs        []string `json:"ids"`
	QueryState string   `json:"queryState"`
	Position   int      `json:"position"`
	Total      int      `json:"total"`
}
type SetResult struct {
	Created map[string]struct {
		ID string `json:"id"`
	} `json:"created"`
	Updated   map[string]any `json:"updated"`
	Destroyed []string       `json:"destroyed"`
	NewState  string         `json:"newState"`
}

func (c *Client) Mailboxes(ctx context.Context) ([]Mailbox, error) {
	var r GetResult[Mailbox]
	err := c.Call(ctx, Mail, "Mailbox/get", nil, &r)
	slices.SortFunc(r.List, func(a, b Mailbox) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Name, b.Name)
	})
	return r.List, err
}
func (c *Client) ResolveMailbox(ctx context.Context, value string) (Mailbox, error) {
	boxes, err := c.Mailboxes(ctx)
	if err != nil {
		return Mailbox{}, err
	}
	for _, b := range boxes {
		if b.ID == value {
			return b, nil
		}
	}
	var matches []Mailbox
	for _, b := range boxes {
		if strings.EqualFold(b.Name, value) || b.Role != "" && strings.EqualFold(b.Role, value) {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Mailbox{}, apierr.ErrUsage("ambiguous mailbox name; use an ID")
	}
	return Mailbox{}, apierr.ErrNotFound("mailbox", value)
}
func (c *Client) RoleMailbox(ctx context.Context, role string) (Mailbox, error) {
	boxes, err := c.Mailboxes(ctx)
	if err != nil {
		return Mailbox{}, err
	}
	for _, b := range boxes {
		if b.Role == role {
			return b, nil
		}
	}
	return Mailbox{}, apierr.ErrNotFound("mailbox role", role)
}
func (c *Client) Identities(ctx context.Context) ([]Identity, error) {
	var r GetResult[Identity]
	err := c.Call(ctx, Submission, "Identity/get", nil, &r)
	return r.List, err
}
func (c *Client) ResolveIdentity(ctx context.Context, value string) (Identity, error) {
	ids, err := c.Identities(ctx)
	if err != nil {
		return Identity{}, err
	}
	for _, i := range ids {
		if i.ID == value || strings.EqualFold(i.Email, value) {
			if strings.HasPrefix(i.Email, "*@") {
				return Identity{}, apierr.ErrUsage("wildcard identity requires a concrete --from email address")
			}
			return i, nil
		}
	}
	addresses, parseErr := ParseAddresses(value)
	if parseErr == nil && len(addresses) == 1 {
		email := addresses[0].Email
		for _, i := range ids {
			if strings.HasPrefix(i.Email, "*@") && strings.HasSuffix(strings.ToLower(email), strings.ToLower(i.Email[1:])) {
				i.Email = email
				return i, nil
			}
		}
	}
	if value == "" && len(ids) == 1 && !strings.HasPrefix(ids[0].Email, "*@") {
		return ids[0], nil
	}
	if value == "" {
		return Identity{}, apierr.ErrUsage("choose --from using 'fastmail identity list'")
	}
	return Identity{}, apierr.ErrNotFound("identity", value)
}

var summaryProperties = []string{"id", "blobId", "threadId", "mailboxIds", "keywords", "from", "to", "subject", "receivedAt", "preview", "hasAttachment"}

func (c *Client) Emails(ctx context.Context, ids []string, bodies bool) ([]Email, error) {
	if len(ids) == 0 {
		return []Email{}, nil
	}
	session, err := c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	maxObjects := 100
	var limits struct {
		MaxObjectsInGet int `json:"maxObjectsInGet"`
	}
	if json.Unmarshal(session.Capabilities[Core], &limits) == nil && limits.MaxObjectsInGet > 0 {
		maxObjects = limits.MaxObjectsInGet
	}
	if len(ids) > maxObjects {
		emails := make([]Email, 0, len(ids))
		for start := 0; start < len(ids); start += maxObjects {
			page, err := c.Emails(ctx, ids[start:min(start+maxObjects, len(ids))], bodies)
			if err != nil {
				return nil, err
			}
			emails = append(emails, page...)
		}
		return emails, nil
	}
	props := slices.Clone(summaryProperties)
	args := map[string]any{"ids": ids}
	if bodies {
		props = append(props, "cc", "bcc", "replyTo", "messageId", "inReplyTo", "references", "textBody", "htmlBody", "attachments", "bodyStructure", "bodyValues")
		args["fetchAllBodyValues"] = true
		args["maxBodyValueBytes"] = 4 << 20
	}
	args["properties"] = props
	var r GetResult[Email]
	if err := c.Call(ctx, Mail, "Email/get", args, &r); err != nil {
		return nil, err
	}
	if len(r.NotFound) > 0 {
		return nil, apierr.ErrNotFound("email", strings.Join(r.NotFound, ","))
	}
	// /get need not preserve the requested order.
	byID := map[string]Email{}
	for _, e := range r.List {
		byID[e.ID] = e
	}
	result := make([]Email, 0, len(ids))
	for _, id := range ids {
		if e, ok := byID[id]; ok {
			result = append(result, e)
		} else {
			return nil, apierr.ErrNotFound("email", id)
		}
	}
	return result, nil
}
func (c *Client) ListEmails(ctx context.Context, filter map[string]any, position, limit int) ([]Email, QueryResult, error) {
	var q QueryResult
	err := c.Call(ctx, Mail, "Email/query", map[string]any{"filter": filter, "sort": []any{map[string]any{"property": "receivedAt", "isAscending": false}}, "position": position, "limit": limit, "calculateTotal": true}, &q)
	if err != nil {
		return nil, q, err
	}
	emails, err := c.Emails(ctx, q.IDs, false)
	return emails, q, err
}
func (c *Client) Thread(ctx context.Context, id string) ([]Email, error) {
	var r GetResult[struct {
		ID       string   `json:"id"`
		EmailIDs []string `json:"emailIds"`
	}]
	if err := c.Call(ctx, Mail, "Thread/get", map[string]any{"ids": []string{id}}, &r); err != nil {
		return nil, err
	}
	if len(r.List) != 1 {
		return nil, apierr.ErrNotFound("thread", id)
	}
	return c.Emails(ctx, r.List[0].EmailIDs, true)
}
func PatchPath(s string) string { return strings.NewReplacer("~", "~0", "/", "~1").Replace(s) }
func (c *Client) UpdateEmails(ctx context.Context, ids []string, patch map[string]any) error {
	update := map[string]any{}
	for _, id := range ids {
		update[id] = patch
	}
	return c.Call(ctx, Mail, "Email/set", map[string]any{"update": update}, nil)
}
func (c *Client) MoveEmails(ctx context.Context, ids []string, box string, copyOnly bool) error {
	target, err := c.ResolveMailbox(ctx, box)
	if err != nil {
		return err
	}
	patch := map[string]any{"mailboxIds": map[string]bool{target.ID: true}}
	if copyOnly {
		patch = map[string]any{"mailboxIds/" + PatchPath(target.ID): true}
	}
	return c.UpdateEmails(ctx, ids, patch)
}

type Compose struct {
	From                  string
	To, CC, BCC           []Address
	Subject, Body         string
	HTML                  bool
	Attachments           []BodyPart
	InReplyTo, References []string
	Structure             *BodyPart
	ReplyTo               []Address
	MailboxIDs, Keywords  map[string]bool
}

func ParseAddresses(value string) ([]Address, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return nil, apierr.ErrUsage("invalid email address: " + err.Error())
	}
	result := make([]Address, 0, len(parsed))
	for _, a := range parsed {
		if strings.ContainsAny(a.Address, "\r\n") {
			return nil, apierr.ErrUsage("invalid address")
		}
		result = append(result, Address{Name: a.Name, Email: a.Address})
	}
	return result, nil
}
func (c *Client) CreateDraft(ctx context.Context, m Compose) (string, error) {
	if strings.ContainsAny(m.Subject, "\r\n") {
		return "", apierr.ErrUsage("subject must be one line")
	}
	identity, err := c.ResolveIdentity(ctx, m.From)
	if err != nil {
		return "", err
	}
	box, err := c.RoleMailbox(ctx, "drafts")
	if err != nil {
		return "", err
	}
	object := m.Object()
	object["from"] = []Address{{Name: identity.Name, Email: identity.Email}}
	if len(m.MailboxIDs) > 0 {
		object["mailboxIds"] = m.MailboxIDs
	} else {
		object["mailboxIds"] = map[string]bool{box.ID: true}
	}
	if m.Keywords == nil {
		m.Keywords = map[string]bool{"$seen": true}
	}
	m.Keywords["$draft"] = true
	object["keywords"] = m.Keywords
	var r SetResult
	err = c.Call(ctx, Mail, "Email/set", map[string]any{"create": map[string]any{"draft": object}}, &r)
	if err != nil {
		return "", err
	}
	id := r.Created["draft"].ID
	if id == "" {
		return "", apierr.ErrAPI(0, "Email/set did not return a draft ID")
	}
	return id, nil
}
func (m Compose) Object() map[string]any {
	media := "text/plain"
	prop := "textBody"
	if m.HTML {
		media = "text/html"
		prop = "htmlBody"
	}
	obj := map[string]any{"to": m.To, "cc": m.CC, "bcc": m.BCC, "subject": m.Subject, prop: []any{map[string]any{"partId": "body", "type": media}}, "bodyValues": map[string]any{"body": map[string]any{"value": m.Body, "isTruncated": false}}}
	if m.Structure != nil {
		delete(obj, "textBody")
		delete(obj, "htmlBody")
		delete(obj, "attachments")
		delete(obj, "bodyValues")
		obj["bodyStructure"] = blobPart(*m.Structure)
	} else if len(m.Attachments) > 0 {
		parts := make([]BodyPart, len(m.Attachments))
		for i, part := range m.Attachments {
			parts[i] = blobPart(part)
		}
		obj["attachments"] = parts
	}
	if m.ReplyTo != nil {
		obj["replyTo"] = m.ReplyTo
	}
	if m.InReplyTo != nil {
		obj["inReplyTo"] = m.InReplyTo
	}
	if m.References != nil {
		obj["references"] = m.References
	}
	return obj
}
func (c *Client) SendDraft(ctx context.Context, id, from string) (string, error) {
	emails, err := c.Emails(ctx, []string{id}, true)
	if err != nil {
		return "", err
	}
	e := emails[0]
	if !e.Keywords["$draft"] {
		return "", apierr.ErrUsage("email is not a draft; refusing to submit it again")
	}
	if len(e.To)+len(e.CC)+len(e.BCC) == 0 {
		return "", apierr.ErrUsage("draft has no recipients")
	}
	if from == "" && len(e.From) > 0 {
		from = e.From[0].Email
	}
	identity, err := c.ResolveIdentity(ctx, from)
	if err != nil {
		return "", err
	}
	if len(e.From) != 1 || !strings.EqualFold(e.From[0].Email, identity.Email) {
		return "", apierr.ErrUsage("submission identity must match the draft From address")
	}
	sent, err := c.RoleMailbox(ctx, "sent")
	if err != nil {
		return "", err
	}
	drafts, err := c.RoleMailbox(ctx, "drafts")
	if err != nil {
		return "", err
	}
	var r SetResult
	args := map[string]any{"create": map[string]any{"send": map[string]any{"emailId": id, "identityId": identity.ID}}, "onSuccessUpdateEmail": map[string]any{"#send": map[string]any{"keywords/$draft": nil, "mailboxIds/" + PatchPath(drafts.ID): nil, "mailboxIds/" + PatchPath(sent.ID): true}}}
	if err = c.Call(ctx, Submission, "EmailSubmission/set", args, &r); err != nil {
		return "", &apierr.Error{Code: apierr.AsError(err).Code, Message: err.Error(), Hint: "Inspect draft and Sent before retrying; a failed response may follow a successful submission.", Meta: map[string]any{"draftId": id}}
	}
	submission := r.Created["send"].ID
	if submission == "" {
		return "", apierr.ErrAPI(0, "submission response has no ID; inspect Sent before retrying")
	}
	return submission, nil
}
func (c *Client) Archive(ctx context.Context, ids []string) error {
	inbox, err := c.RoleMailbox(ctx, "inbox")
	if err != nil {
		return err
	}
	// Ensure each message retains a mailbox even on servers without an archive role.
	archive, err := c.RoleMailbox(ctx, "archive")
	if err != nil {
		return err
	}
	return c.UpdateEmails(ctx, ids, map[string]any{"mailboxIds/" + PatchPath(inbox.ID): nil, "mailboxIds/" + PatchPath(archive.ID): true})
}
func Date(value string) (string, error) {
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.Format(time.RFC3339), nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", apierr.ErrUsage(fmt.Sprintf("invalid date %q; use YYYY-MM-DD or RFC3339", value))
	}
	return t.UTC().Format(time.RFC3339), nil
}

// Returned body parts include both partId and blobId. RFC 8621 permits only
// one when creating a message that reuses the existing blob.
func blobPart(part BodyPart) BodyPart {
	if part.BlobID != "" {
		part.PartID = ""
	}
	if part.SubParts != nil {
		children := make([]BodyPart, len(part.SubParts))
		for i, p := range part.SubParts {
			children[i] = blobPart(p)
		}
		part.SubParts = children
	}
	return part
}
