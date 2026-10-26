package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

func TestEmptyMailboxNavigationDoesNotCreateNegativeSelection(t *testing.T) {
	m := New(t.Context(), nil)
	m.loading = false
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	if m.index < 0 {
		t.Fatal(m.index)
	}
	next, _ = m.Update(emailsMsg{emails: []jmap.Email{{ID: "one"}}, total: 1})
	m = next.(Model)
	if m.index != 0 {
		t.Fatal(m.index)
	}
}
func TestViewAndBodyStripTerminalControls(t *testing.T) {
	m := New(t.Context(), nil)
	m.loading = false
	m.boxes = []jmap.Mailbox{{Name: "Inbox\x1b[2J"}}
	m.emails = []jmap.Email{{ID: "one", Subject: "Review\x1b]52;c;bad\x07"}}
	view := m.View()
	if strings.Contains(view.Content, "\x1b]52") || strings.Contains(view.Content, "\x1b[2J") {
		t.Fatal("unsafe terminal content")
	}
	updated, _ := m.Update(readMsg{email: jmap.Email{Subject: "Hello\x1b[2J", TextBody: []jmap.BodyPart{{PartID: "text"}}, BodyValues: map[string]jmap.BodyValue{"text": {Value: "Meeting\x1b]52;c;bad\x07"}}}})
	m = updated.(Model)
	if !m.reading || strings.ContainsAny(m.viewport.GetContent(), "\x1b\x07") {
		t.Fatal(m.viewport.GetContent())
	}
}
func TestEscapeReturnsToMailbox(t *testing.T) {
	m := New(t.Context(), nil)
	m.loading = false
	m.reading = true
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(Model).reading {
		t.Fatal("still reading")
	}
}
