package cmd

import (
	"strings"
	"testing"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

func TestContactCreationAndUpdatePreserveOtherAddresses(t *testing.T) {
	isolated(t)
	created, updated := false, false
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "AddressBook/get":
			return map[string]any{"list": []jmap.AddressBook{{ID: "book", IsDefault: true, MyRights: map[string]bool{"mayWrite": true}}}}
		case "ContactCard/get":
			return map[string]any{"list": []map[string]any{{"id": "card", "emails": map[string]any{"work": map[string]any{"address": "alice.work@example.com"}}}}}
		case "ContactCard/set":
			if create, ok := m.Args["create"].(map[string]any); ok {
				created = true
				obj := create["contact"].(map[string]any)
				if obj["@type"] != "Card" || obj["version"] != "1.0" || obj["addressBookIds"].(map[string]any)["book"] != true || obj["uid"] == "" {
					t.Errorf("invalid card=%v", obj)
				}
				return map[string]any{"created": map[string]any{"contact": map[string]string{"id": "card"}}}
			}
			updated = true
			patch := m.Args["update"].(map[string]any)["card"].(map[string]any)
			emails := patch["emails"].(map[string]any)
			if emails["work"].(map[string]any)["address"] != "alice.work@example.com" || emails["email"].(map[string]any)["address"] != "alice.private@example.com" {
				t.Error("contact address lost", patch)
			}
			if patch["name"].(map[string]any)["full"] != "Alice Example" {
				t.Error("missing name object", patch)
			}
			return map[string]any{"updated": map[string]any{"card": nil}}
		default:
			t.Errorf("unexpected method %s", m.Name)
			return nil
		}
	})
	if out, err := run(t, "contact", "add", "--name", "Alice Example", "--email", "alice.private@example.com"); err != nil || !strings.Contains(out, "card") {
		t.Fatal(out, err)
	}
	if _, err := run(t, "contact", "update", "card", "--name", "Alice Example", "--email", "alice.private@example.com"); err != nil {
		t.Fatal(err)
	}
	if !created || !updated {
		t.Fatal(created, updated)
	}
}
func TestContactSearchUsesServerPagination(t *testing.T) {
	isolated(t)
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "ContactCard/query":
			if m.Args["filter"].(map[string]any)["text"] != "Alice Example" || m.Args["position"] != float64(10) {
				t.Error(m.Args)
			}
			return map[string]any{"ids": []string{"card"}, "total": 12}
		case "ContactCard/get":
			return map[string]any{"list": []map[string]any{{"id": "card", "name": map[string]string{"full": "Alice Example"}}}}
		default:
			t.Error(m.Name)
			return nil
		}
	})
	out, err := run(t, "contact", "search", "Alice", "Example", "--position", "10", "--ids-only")
	if err != nil || out != "card\n" {
		t.Fatal(out, err)
	}
}
func TestMaskedEmailAndVacationUseDedicatedCapabilities(t *testing.T) {
	isolated(t)
	masked, vacation := false, false
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "MaskedEmail/set":
			masked = true
			obj := m.Args["create"].(map[string]any)["masked"].(map[string]any)
			if obj["state"] != "enabled" || obj["forDomain"] != "https://example.com" {
				t.Error(obj)
			}
			return map[string]any{"created": map[string]any{"masked": map[string]string{"id": "masked"}}}
		case "VacationResponse/set":
			vacation = true
			obj := m.Args["update"].(map[string]any)["singleton"].(map[string]any)
			if obj["isEnabled"] != true || obj["fromDate"] != "2026-10-26T00:00:00Z" || obj["toDate"] != "2026-11-02T00:00:00Z" {
				t.Error(obj)
			}
			return map[string]any{"updated": map[string]any{"singleton": nil}}
		default:
			t.Error(m.Name)
			return nil
		}
	})
	if _, err := run(t, "masked-email", "create", "--for-domain", "https://example.com", "--description", "Shopping"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "vacation", "set", "-m", "Back on Monday.", "--from", "2026-10-26", "--until", "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	if !masked || !vacation {
		t.Fatal(masked, vacation)
	}
}
