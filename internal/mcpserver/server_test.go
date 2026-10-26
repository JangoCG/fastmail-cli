package mcpserver

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

func TestMCPExposesReadsAndDraftsWithoutSendingTools(t *testing.T) {
	var calls []string
	c := testutil.Client(t, func(m testutil.Method) any {
		calls = append(calls, m.Name)
		switch m.Name {
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "draftbox", Role: "drafts"}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Email/set":
			return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "saved"}}}
		default:
			t.Errorf("unexpected %s", m.Name)
			return nil
		}
	})
	if _, err := c.Discover(t.Context()); err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	server := New(c, "")
	ss, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	listing, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(listing.Tools))
	for _, tool := range listing.Tools {
		names = append(names, tool.Name)
		if strings.Contains(tool.Name, "send") || strings.Contains(tool.Name, "delete") {
			t.Fatal("unsafe MCP tool", tool.Name)
		}
	}
	if len(names) != 6 || !slices.Contains(names, "draft_create") || !slices.Contains(names, "email_read") {
		t.Fatal(names)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "draft_create", Arguments: map[string]any{"to": "bob@example.com", "subject": "Lunch", "body": "Noon?"}})
	if err != nil || res.IsError {
		t.Fatal(res, err)
	}
	if slices.Contains(calls, "EmailSubmission/set") {
		t.Fatal("MCP sent mail")
	}
}
