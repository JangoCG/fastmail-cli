package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/config"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

func isolated(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FASTMAIL_NO_KEYRING", "1")
	t.Setenv("FASTMAIL_TOKEN", "")
	t.Setenv("FASTMAIL_ACCOUNT", "")
	t.Setenv("FASTMAIL_SESSION_URL", "")
	t.Setenv("FASTMAIL_DAV_USERNAME", "")
	t.Setenv("FASTMAIL_DAV_PASSWORD", "")
}
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRoot(t.Context())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}
func authenticated(t *testing.T, handler testutil.Handler) {
	t.Helper()
	s := testutil.Server(t, handler)
	t.Setenv("FASTMAIL_TOKEN", "test-token")
	t.Setenv("FASTMAIL_SESSION_URL", s.URL+"/session")
}
func TestNoAuthNeededForHelpAndConfig(t *testing.T) {
	isolated(t)
	for _, args := range [][]string{{"--help"}, {"version", "--json"}, {"config", "show"}, {"completion", "bash"}} {
		if out, err := run(t, args...); err != nil || out == "" {
			t.Fatalf("%v: %s %v", args, out, err)
		}
	}
	_, err := run(t, "mailbox", "list")
	if apierr.AsError(err).Code != apierr.CodeAuth {
		t.Fatal(err)
	}
}
func TestMailListingOutputFiltersAndPagination(t *testing.T) {
	isolated(t)
	queries := 0
	authenticated(t, func(m testutil.Method) any {
		switch m.Name {
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "inbox", Name: "Inbox", Role: "inbox"}}}
		case "Email/query":
			queries++
			filter := m.Args["filter"].(map[string]any)
			if filter["inMailbox"] != "inbox" || filter["notKeyword"] != "$seen" || filter["after"] != "2026-10-01T00:00:00Z" {
				t.Errorf("filter=%v", filter)
			}
			if m.Args["position"] == float64(0) {
				return map[string]any{"ids": []string{"first"}, "total": 2}
			}
			return map[string]any{"ids": []string{"second"}, "total": 2}
		case "Email/get":
			ids := m.Args["ids"].([]any)
			return map[string]any{"list": []any{map[string]any{"id": ids[0], "subject": "Lunch"}}}
		default:
			t.Errorf("unexpected %s", m.Name)
			return nil
		}
	})
	out, err := run(t, "email", "list", "--mailbox", "inbox", "--unread", "--after", "2026-10-01", "--limit", "1", "--all", "--ids-only")
	if err != nil || out != "first\nsecond\n" || queries != 2 {
		t.Fatalf("%q queries=%d err=%v", out, queries, err)
	}
	out, err = run(t, "email", "list", "--mailbox", "inbox", "--unread", "--after", "2026-10-01", "--jq", ".data[].subject")
	if err != nil || out != "Lunch\n" {
		t.Fatal(out, err)
	}
}
func TestInvalidArgumentsDoNotHitNetwork(t *testing.T) {
	isolated(t)
	for _, args := range [][]string{{"email", "list", "--limit", "0"}, {"email", "delete", "email"}, {"contact", "delete", "contact"}, {"event", "delete", "/calendar/event.ics"}, {"watch", "--interval", "0s"}, {"email", "read"}, {"compose", "extra"}, {"email", "list", "--json", "--count"}} {
		if _, err := run(t, args...); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
func draftHandler(t *testing.T, calls *[]string) testutil.Handler {
	return func(m testutil.Method) any {
		*calls = append(*calls, m.Name)
		switch m.Name {
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "draftbox", Role: "drafts"}, {ID: "sentbox", Role: "sent"}}}
		case "Email/set":
			if create, ok := m.Args["create"].(map[string]any); ok {
				obj := create["draft"].(map[string]any)
				if obj["subject"] != "Lunch" {
					t.Errorf("subject=%v", obj["subject"])
				}
				return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "draft"}}}
			}
			return map[string]any{"updated": map[string]any{"draft": nil}}
		case "Email/get":
			return map[string]any{"list": []jmap.Email{{ID: "draft", From: []jmap.Address{{Email: "alice@example.com"}}, To: []jmap.Address{{Email: "bob@example.com"}}, Keywords: map[string]bool{"$draft": true}}}}
		case "EmailSubmission/set":
			return map[string]any{"created": map[string]any{"send": map[string]string{"id": "submission"}}}
		default:
			t.Errorf("unexpected %s", m.Name)
			return nil
		}
	}
}
func TestComposeDefaultDraftAndExplicitSend(t *testing.T) {
	for _, send := range []bool{false, true} {
		t.Run(map[bool]string{true: "send", false: "draft"}[send], func(t *testing.T) {
			isolated(t)
			var calls []string
			authenticated(t, draftHandler(t, &calls))
			args := []string{"compose", "--to", "bob@example.com", "--subject", "Lunch", "-m", "Noon?", "--json"}
			if send {
				args = append(args, "--send")
			}
			out, err := run(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			var resp struct {
				OK   bool
				Data struct {
					ID   string
					Sent bool
				}
			}
			if err = json.Unmarshal([]byte(out), &resp); err != nil {
				t.Fatal(err)
			}
			if !resp.OK || resp.Data.ID != "draft" || resp.Data.Sent != send || slices.Contains(calls, "EmailSubmission/set") != send {
				t.Fatalf("result=%s calls=%v", out, calls)
			}
		})
	}
}
func TestReadAndTerminalOutputAreSanitized(t *testing.T) {
	isolated(t)
	authenticated(t, func(m testutil.Method) any {
		return map[string]any{"list": []jmap.Email{{ID: "message", Subject: "Quarterly\x1b[2J report", TextBody: []jmap.BodyPart{{PartID: "text"}}, BodyValues: map[string]jmap.BodyValue{"text": {Value: "Hello\x1b]52;c;injected\x07 world"}}}}}
	})
	out, err := run(t, "email", "read", "message", "--styled")
	if err != nil || strings.ContainsAny(out, "\x1b\x07") || !strings.Contains(out, "Quarterly") {
		t.Fatal(out, err)
	}
}
func TestDraftReplacementKeepsOriginalOnCreationFailure(t *testing.T) {
	isolated(t)
	var calls []string
	authenticated(t, func(m testutil.Method) any {
		calls = append(calls, m.Name)
		switch m.Name {
		case "Email/get":
			return map[string]any{"list": []jmap.Email{{ID: "original", From: []jmap.Address{{Email: "alice@example.com"}}, Keywords: map[string]bool{"$draft": true}}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "draftbox", Role: "drafts"}}}
		case "Email/set":
			if m.Args["destroy"] != nil {
				t.Error("destroyed original after failed creation")
			}
			return map[string]any{"notCreated": map[string]any{"draft": map[string]string{"type": "overQuota"}}}
		default:
			t.Errorf("unexpected %s", m.Name)
			return nil
		}
	})
	_, err := run(t, "draft", "update", "original", "-m", "New body")
	if err == nil {
		t.Fatal("expected overQuota")
	}
}
func TestWatchStartsWithEmptyGetAndDrainsChanges(t *testing.T) {
	isolated(t)
	changes := 0
	authenticated(t, func(m testutil.Method) any {
		if m.Name == "Email/get" {
			if ids, ok := m.Args["ids"].([]any); !ok || len(ids) != 0 {
				t.Errorf("baseline downloaded mail: %v", m.Args)
			}
			return map[string]any{"state": "baseline", "list": []any{}}
		}
		if m.Name != "Email/changes" {
			t.Errorf("unexpected %s", m.Name)
		}
		changes++
		if changes == 1 {
			return map[string]any{"newState": "page2", "hasMoreChanges": true, "created": []string{"new"}}
		}
		if m.Args["sinceState"] != "page2" {
			t.Errorf("cursor not advanced: %v", m.Args)
		}
		return map[string]any{"newState": "done", "hasMoreChanges": false, "destroyed": []string{"old"}}
	})
	out, err := run(t, "watch", "--once")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 || !strings.Contains(lines[0], "baseline") || !strings.Contains(lines[1], "created") || !strings.Contains(lines[4], "done") {
		t.Fatal(out)
	}
}
func TestAuthLoginFromStdinPersistsOnlyValidatedToken(t *testing.T) {
	isolated(t)
	s := testutil.Server(t, func(m testutil.Method) any { t.Errorf("login should only use discovery, got %s", m.Name); return nil })
	t.Setenv("FASTMAIL_SESSION_URL", s.URL+"/session")
	root := NewRoot(t.Context())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader("test-token\n"))
	root.SetArgs([]string{"auth", "login", "--token-stdin"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "test-token") {
		t.Fatal("token leaked")
	}
	cfg, err := config.Load()
	if err != nil || cfg.SessionURL != s.URL+"/session" {
		t.Fatal(cfg, err)
	}
	out.Reset()
	result, err := run(t, "auth", "status")
	if err != nil || !strings.Contains(result, "alice@example.com") {
		t.Fatal(result, err)
	}
	if _, err = run(t, "auth", "logout"); err != nil {
		t.Fatal(err)
	}
	if _, err = run(t, "auth", "status"); apierr.AsError(err).Code != apierr.CodeAuth {
		t.Fatal(err)
	}
}
func TestSkillInstallRefusesUnmanagedDirectory(t *testing.T) {
	isolated(t)
	dest := filepath.Join(t.TempDir(), "fastmail")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dest, "SKILL.md")
	if err := os.WriteFile(file, []byte("user-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "setup", "codex", "--dir", dest); err == nil {
		t.Fatal("unmanaged directory overwritten")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "user-owned" {
		t.Fatal(string(data))
	}
	fresh := filepath.Join(t.TempDir(), "fastmail")
	if _, err := run(t, "setup", "codex", "--dir", fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "setup", "codex", "--dir", fresh); err != nil {
		t.Fatal("refresh failed:", err)
	}
}

var updateCommands = flag.Bool("update-commands", false, "Update docs/commands.txt")

func TestCommandInventory(t *testing.T) {
	var lines []string
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		lines = append(lines, cmd.CommandPath()+"\t"+cmd.Use+"\t"+cmd.Short)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(NewRoot(t.Context()))
	slices.Sort(lines)
	snapshot := strings.Join(lines, "\n") + "\n"
	path := filepath.Join("..", "..", "docs", "commands.txt")
	if *updateCommands {
		if err := os.WriteFile(path, []byte(snapshot), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != snapshot {
		t.Fatal("CLI inventory changed; regenerate with go test ./internal/cmd -run TestCommandInventory -update-commands")
	}
}
func TestCancellationEndsWatch(t *testing.T) {
	isolated(t)
	authenticated(t, func(m testutil.Method) any {
		if m.Name == "Email/get" {
			return map[string]any{"state": "baseline"}
		}
		return map[string]any{"newState": "baseline"}
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	root := NewRoot(t.Context())
	root.SetArgs([]string{"watch"})
	if err := root.ExecuteContext(ctx); err == nil {
		t.Fatal("discovery must observe cancellation")
	}
}
