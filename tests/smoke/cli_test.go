package smoke_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/testutil"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fastmail-smoke-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "fastmail")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/fastmail")
	cmd.Dir = filepath.Join("..", "..")
	data, err := cmd.CombinedOutput()
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build CLI: %v\n%s", err, data)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
func invoke(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return out.String(), errout.String(), code
}
func isolated(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("FASTMAIL_NO_KEYRING", "1")
	for _, key := range []string{"FASTMAIL_TOKEN", "FASTMAIL_SESSION_URL", "FASTMAIL_ACCOUNT", "FASTMAIL_DAV_USERNAME", "FASTMAIL_DAV_PASSWORD"} {
		t.Setenv(key, "")
	}
}
func TestBuiltCLIUsesFastmailProtocolAndExitCodes(t *testing.T) {
	isolated(t)
	if out, _, code := invoke(t, "--help"); code != 0 || !strings.Contains(out, "Fastmail") || strings.Contains(out, "screener") {
		t.Fatal(out, code)
	}
	if out, errout, code := invoke(t, "email", "list", "--json"); code != 3 || out != "" || !strings.Contains(errout, `"code": "auth"`) {
		t.Fatal(out, errout, code)
	}
	var submissions atomic.Int32
	s := testutil.Server(t, func(m testutil.Method) any {
		switch m.Name {
		case "Mailbox/get":
			return map[string]any{"list": []jmap.Mailbox{{ID: "inbox", Name: "Inbox", Role: "inbox"}, {ID: "draftbox", Role: "drafts"}, {ID: "sentbox", Role: "sent"}}}
		case "Email/query":
			return map[string]any{"ids": []string{"email"}, "total": 1}
		case "Email/get":
			return map[string]any{"list": []jmap.Email{{ID: "email", Subject: "Lunch", From: []jmap.Address{{Email: "alice@example.com"}}, To: []jmap.Address{{Email: "bob@example.com"}}, Keywords: map[string]bool{"$draft": true}, TextBody: []jmap.BodyPart{{PartID: "body", Type: "text/plain"}}, BodyValues: map[string]jmap.BodyValue{"body": {Value: "Noon works."}}}}}
		case "Identity/get":
			return map[string]any{"list": []jmap.Identity{{ID: "identity", Email: "alice@example.com"}}}
		case "Email/set":
			return map[string]any{"created": map[string]any{"draft": map[string]string{"id": "email"}}}
		case "EmailSubmission/set":
			submissions.Add(1)
			return map[string]any{"created": map[string]any{"send": map[string]string{"id": "submission"}}}
		default:
			t.Errorf("unexpected %s", m.Name)
			return nil
		}
	})
	t.Setenv("FASTMAIL_TOKEN", "test-token")
	t.Setenv("FASTMAIL_SESSION_URL", s.URL+"/session")
	if out, errout, code := invoke(t, "email", "list", "--mailbox", "inbox", "--jq", ".data[].subject"); code != 0 || out != "Lunch\n" {
		t.Fatal(out, errout, code)
	}
	if out, errout, code := invoke(t, "email", "read", "email", "--markdown"); code != 0 || !strings.Contains(out, "Noon works.") {
		t.Fatal(out, errout, code)
	}
	args := []string{"compose", "--to", "bob@example.com", "--subject", "Lunch", "-m", "Noon?", "--json"}
	out, errout, code := invoke(t, args...)
	if code != 0 || !json.Valid([]byte(out)) || submissions.Load() != 0 {
		t.Fatal(out, errout, code)
	}
	if out, errout, code = invoke(t, append(args, "--send")...); code != 0 || submissions.Load() != 1 || !strings.Contains(out, `"sent": true`) {
		t.Fatal(out, errout, code)
	}
}
