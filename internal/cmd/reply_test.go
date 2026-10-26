package cmd

import (
	"testing"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

func TestReplyAllAvoidsSelfAndPreservesThreadReferences(t *testing.T) {
	isolated(t)
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "Email/get":
			return map[string]any{"list": []jmap.Email{{ID: "original", From: []jmap.Address{{Email: "bob@example.com"}}, To: []jmap.Address{{Email: "alice@example.com"}, {Email: "carol@example.com"}}, CC: []jmap.Address{{Email: "carol@example.com"}}, ReplyTo: []jmap.Address{{Email: "replies@example.com"}}, Subject: "Lunch", MessageID: []string{"message@example.com"}, References: []string{"earlier@example.com"}}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "drafts", Role: "drafts"}}}
		case "Email/set":
			obj := m.Args["create"].(map[string]any)["draft"].(map[string]any)
			to := obj["to"].([]any)
			cc := obj["cc"].([]any)
			if len(to) != 1 || to[0].(map[string]any)["email"] != "replies@example.com" || len(cc) != 1 || cc[0].(map[string]any)["email"] != "carol@example.com" {
				t.Errorf("wrong reply recipients: %v", obj)
			}
			refs := obj["references"].([]any)
			if len(refs) != 2 || obj["inReplyTo"].([]any)[0] != "message@example.com" {
				t.Error("lost threading", obj)
			}
			return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "reply"}}}
		default:
			t.Error(m.Name)
			return nil
		}
	})
	if _, err := run(t, "reply", "original", "--all", "-m", "Noon works."); err != nil {
		t.Fatal(err)
	}
}
func TestReplyToSentMessageContinuesToOriginalRecipient(t *testing.T) {
	isolated(t)
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "Email/get":
			return map[string]any{"list": []jmap.Email{{ID: "sent", From: []jmap.Address{{Email: "alice@example.com"}}, To: []jmap.Address{{Email: "bob@example.com"}}, Subject: "Lunch"}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "drafts", Role: "drafts"}}}
		case "Email/set":
			obj := m.Args["create"].(map[string]any)["draft"].(map[string]any)
			if obj["to"].([]any)[0].(map[string]any)["email"] != "bob@example.com" {
				t.Error("replied to self", obj)
			}
			return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "reply"}}}
		default:
			t.Error(m.Name)
			return nil
		}
	})
	if _, err := run(t, "reply", "sent", "-m", "Also, the cafe opens at noon."); err != nil {
		t.Fatal(err)
	}
}
