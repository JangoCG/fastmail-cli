package jmap_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

func TestListPreservesQueryOrderAndScope(t *testing.T) {
	c := testutil.Client(t, func(m testutil.Method) any {
		switch m.Name {
		case "Email/query":
			if m.Args["position"] != float64(50) || m.Args["limit"] != float64(10) {
				t.Errorf("pagination: %v", m.Args)
			}
			filter := m.Args["filter"].(map[string]any)
			if filter["from"] != "alice@example.com" {
				t.Errorf("filter: %v", filter)
			}
			return map[string]any{"ids": []string{"second", "first"}, "total": 73, "position": 50, "queryState": "opaque"}
		case "Email/get":
			return map[string]any{"list": []any{map[string]any{"id": "first", "subject": "one"}, map[string]any{"id": "second", "subject": "two"}}}
		default:
			t.Fatalf("unexpected %s", m.Name)
			return nil
		}
	})
	list, q, err := c.ListEmails(t.Context(), map[string]any{"from": "alice@example.com"}, 50, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "second" || list[1].ID != "first" || q.Total != 73 {
		t.Fatalf("result=%+v %+v", list, q)
	}
}
func TestSetObjectFailuresAreErrors(t *testing.T) {
	for _, kind := range []string{"notCreated", "notUpdated", "notDestroyed"} {
		t.Run(kind, func(t *testing.T) {
			c := testutil.Client(t, func(m testutil.Method) any {
				return map[string]any{kind: map[string]any{"email": map[string]any{"type": "forbidden", "description": "read only"}}}
			})
			err := c.Call(t.Context(), jmap.Mail, "Email/set", map[string]any{}, nil)
			var e *apierr.Error
			if !errors.As(err, &e) || e.Code != apierr.CodeForbidden || e.Meta["id"] != "email" {
				t.Fatalf("error=%+v", err)
			}
		})
	}
}
func TestMethodAndHTTPErrorsNeverRetry(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "https://example.org/")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "secret server details")
			}))
			defer s.Close()
			c := jmap.New("sensitive-token", s.URL, "")
			_, err := c.Discover(t.Context())
			if err == nil || calls != 1 || strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "secret server") {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
	var server *httptest.Server
	server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]any{jmap.Core: map[string]any{}, jmap.Mail: map[string]any{}}, "accounts": map[string]any{"a": map[string]any{"accountCapabilities": map[string]any{jmap.Mail: map[string]any{}}}}, "primaryAccounts": map[string]string{jmap.Mail: "a"}, "apiUrl": server.URL})
			return
		}
		_, _ = io.WriteString(w, `{"methodResponses":[["error",{"type":"cannotCalculateChanges","description":"expired cursor"},"0"]]}`)
	}))
	server.Start()
	defer server.Close()
	_, err := jmap.New("token", server.URL, "").Changes(t.Context(), "expired")
	if err == nil || !strings.Contains(err.Error(), "cannotCalculateChanges") {
		t.Fatal(err)
	}
}
func TestCapabilitiesAndSharedAccountSelection(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": map[string]any{jmap.Core: map[string]any{}, jmap.Mail: map[string]any{}, jmap.Contacts: map[string]any{}}, "primaryAccounts": map[string]string{jmap.Mail: "personal"}, "accounts": map[string]any{"personal": map[string]any{"accountCapabilities": map[string]any{jmap.Mail: map[string]any{}}}, "shared": map[string]any{"accountCapabilities": map[string]any{jmap.Mail: map[string]any{}, jmap.Contacts: map[string]any{}}}}, "apiUrl": server.URL})
	}))
	server.Start()
	defer server.Close()
	c := jmap.New("token", server.URL, "")
	id, err := c.Account(t.Context(), jmap.Mail)
	if err != nil || id != "personal" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	id, err = c.Account(t.Context(), jmap.Contacts)
	if err != nil || id != "shared" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	_, err = c.Account(t.Context(), jmap.Masked)
	if apierr.AsError(err).Code != apierr.CodeForbidden {
		t.Fatalf("error=%v", err)
	}
	c.AccountID = "shared"
	id, err = c.Account(t.Context(), jmap.Mail)
	if err != nil || id != "shared" {
		t.Fatal(id, err)
	}
	c.AccountID = "personal"
	_, err = c.Account(t.Context(), jmap.Contacts)
	if apierr.AsError(err).Code != apierr.CodeForbidden {
		t.Fatal(err)
	}
}
func TestMissingEmailsAreNotSilentlyDropped(t *testing.T) {
	c := testutil.Client(t, func(m testutil.Method) any { return map[string]any{"list": []any{}, "notFound": []string{"gone"}} })
	_, err := c.Emails(t.Context(), []string{"gone"}, true)
	if apierr.AsError(err).Code != apierr.CodeNotFound {
		t.Fatal(err)
	}
}
func TestSendDraftUsesSubmissionAndEscapedMailboxPatches(t *testing.T) {
	var sequence []string
	c := testutil.Client(t, func(m testutil.Method) any {
		sequence = append(sequence, m.Name)
		switch m.Name {
		case "Email/get":
			return map[string]any{"list": []any{map[string]any{"id": "draft", "keywords": map[string]bool{"$draft": true}, "from": []jmap.Address{{Email: "alice@example.com"}}, "to": []jmap.Address{{Email: "bob@example.com"}}}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "drafts/~", Role: "drafts"}, {ID: "sent/~", Role: "sent"}}}
		case "EmailSubmission/set":
			if !reflect.DeepEqual(m.Using, []string{jmap.Core, jmap.Submission, jmap.Mail}) {
				t.Errorf("using=%v", m.Using)
			}
			create := m.Args["create"].(map[string]any)["send"].(map[string]any)
			if create["emailId"] != "draft" || create["identityId"] != "identity" {
				t.Errorf("submission=%v", create)
			}
			patch := m.Args["onSuccessUpdateEmail"].(map[string]any)["#send"].(map[string]any)
			if patch["mailboxIds/sent~1~0"] != true {
				t.Errorf("patch=%v", patch)
			}
			if v, ok := patch["mailboxIds/drafts~1~0"]; !ok || v != nil {
				t.Errorf("draft mailbox not removed: %v", patch)
			}
			if v, ok := patch["keywords/$draft"]; !ok || v != nil {
				t.Errorf("draft flag not removed: %v", patch)
			}
			return map[string]any{"created": map[string]any{"send": map[string]string{"id": "submission"}}}
		default:
			t.Fatalf("unexpected %s", m.Name)
			return nil
		}
	})
	id, err := c.SendDraft(t.Context(), "draft", "")
	if err != nil || id != "submission" {
		t.Fatalf("%s %v", id, err)
	}
	if sequence[len(sequence)-1] != "EmailSubmission/set" {
		t.Fatal(sequence)
	}
}
func TestCannotResendNonDraft(t *testing.T) {
	calls := 0
	c := testutil.Client(t, func(m testutil.Method) any {
		calls++
		return map[string]any{"list": []jmap.Email{{ID: "sent", Keywords: map[string]bool{}}}}
	})
	_, err := c.SendDraft(t.Context(), "sent", "")
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
func TestCreateDraftDoesNotSubmit(t *testing.T) {
	c := testutil.Client(t, func(m testutil.Method) any {
		switch m.Name {
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Name: "Alice", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "draftbox", Role: "drafts"}}}
		case "Email/set":
			obj := m.Args["create"].(map[string]any)["draft"].(map[string]any)
			if obj["subject"] != "Lunch" || obj["keywords"].(map[string]any)["$draft"] != true || obj["htmlBody"] == nil {
				t.Errorf("draft=%v", obj)
			}
			return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "saved-draft"}}}
		default:
			t.Fatalf("unexpected request: %s", m.Name)
			return nil
		}
	})
	id, err := c.CreateDraft(t.Context(), jmap.Compose{To: []jmap.Address{{Email: "bob@example.com"}}, Subject: "Lunch", Body: "<p>Noon?</p>", HTML: true})
	if err != nil || id != "saved-draft" {
		t.Fatal(id, err)
	}
}
func TestBodyConversionAndDates(t *testing.T) {
	e := jmap.Email{HTMLBody: []jmap.BodyPart{{PartID: "html"}}, BodyValues: map[string]jmap.BodyValue{"html": {Value: "<p>Hello <strong>Alice</strong>\x1b[2J</p>"}}}
	if body := e.Body(); !strings.Contains(body, "Hello") || strings.Contains(body, "\x1b") {
		t.Fatal(body)
	}
	if d, err := jmap.Date("2026-10-26T14:00:00+01:00"); err != nil || d != "2026-10-26T13:00:00Z" {
		t.Fatal(d, err)
	}
	if _, err := jmap.Date("October 26"); err == nil {
		t.Fatal("invalid date accepted")
	}
	for _, raw := range []string{"http://example.com/session", "https://alice:password@example.com", "file:///tmp/session"} {
		if err := jmap.ValidateURL(raw); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"https://api.fastmail.com/jmap/session", "http://127.0.0.1:1234/session"} {
		if err := jmap.ValidateURL(raw); err != nil {
			t.Fatal(err)
		}
	}
}
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := jmap.New("token", "http://127.0.0.1:1", "")
	if _, err := c.Discover(ctx); err == nil {
		t.Fatal("cancel ignored")
	}
}

func TestReusingBodyBlobsOmitsPartIDsAndPreservesMIMEStructure(t *testing.T) {
	structure := jmap.BodyPart{Type: "multipart/related", SubParts: []jmap.BodyPart{{PartID: "html", BlobID: "html-blob", Type: "text/html"}, {PartID: "image", BlobID: "image-blob", Type: "image/png", CID: "photo@example.com"}}}
	m := jmap.Compose{Body: "ignored", Structure: &structure, Attachments: []jmap.BodyPart{{BlobID: "ignored", Type: "text/plain"}}}
	obj := m.Object()
	for _, key := range []string{"textBody", "htmlBody", "attachments", "bodyValues"} {
		if _, ok := obj[key]; ok {
			t.Errorf("conflicting create property %s", key)
		}
	}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "partId") || !strings.Contains(string(data), `"cid":"photo@example.com"`) {
		t.Fatal(string(data))
	}
	if structure.SubParts[0].PartID != "html" {
		t.Fatal("source structure mutated")
	}
	obj = (jmap.Compose{Attachments: []jmap.BodyPart{{PartID: "old", BlobID: "existing", Type: "application/pdf"}}}).Object()
	data, _ = json.Marshal(obj["attachments"])
	if strings.Contains(string(data), "partId") || !strings.Contains(string(data), "existing") {
		t.Fatal(string(data))
	}
	if v, ok := (jmap.Compose{}).Object()["attachments"]; ok {
		t.Fatalf("empty attachment list should be omitted, got %v", v)
	}
}
func TestLargeEmailReadsAreBatched(t *testing.T) {
	calls := 0
	c := testutil.Client(t, func(m testutil.Method) any {
		calls++
		ids := m.Args["ids"].([]any)
		if len(ids) > 100 {
			t.Errorf("too many objects: %d", len(ids))
		}
		list := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			list = append(list, map[string]any{"id": id})
		}
		return map[string]any{"list": list}
	})
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = strings.Repeat("m", i+1)
	}
	emails, err := c.Emails(t.Context(), ids, false)
	if err != nil || len(emails) != 201 || calls != 3 {
		t.Fatal(len(emails), calls, err)
	}
}

func TestWildcardIdentityRequiresConcreteSender(t *testing.T) {
	c := testutil.Client(t, func(m testutil.Method) any {
		return map[string]any{"list": []jmap.Identity{{ID: "domain", Email: "*@example.com"}}}
	})
	identity, err := c.ResolveIdentity(t.Context(), "alice@example.com")
	if err != nil || identity.Email != "alice@example.com" || identity.ID != "domain" {
		t.Fatal(identity, err)
	}
	if _, err = c.ResolveIdentity(t.Context(), ""); err == nil {
		t.Fatal("wildcard From accepted")
	}
	if _, err = c.ResolveIdentity(t.Context(), "bob@example.org"); err == nil {
		t.Fatal("foreign domain accepted")
	}
}
