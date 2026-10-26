package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/terminal"
)

type Model struct {
	ctx                         context.Context
	client                      *jmap.Client
	boxes                       []jmap.Mailbox
	box, index, position, total int
	emails                      []jmap.Email
	width, height               int
	loading, reading            bool
	status                      string
	viewport                    viewport.Model
}
type boxesMsg struct {
	boxes []jmap.Mailbox
	err   error
}
type emailsMsg struct {
	emails []jmap.Email
	total  int
	err    error
}
type readMsg struct {
	email jmap.Email
	err   error
}
type mutationMsg struct{ err error }
type tickMsg time.Time

func New(ctx context.Context, c *jmap.Client) Model {
	return Model{ctx: ctx, client: c, width: 80, height: 24, loading: true, viewport: viewport.New()}
}
func (m Model) Init() tea.Cmd {
	return func() tea.Msg { boxes, err := m.client.Mailboxes(m.ctx); return boxesMsg{boxes, err} }
}
func (m Model) fetch() tea.Cmd {
	if len(m.boxes) == 0 {
		return nil
	}
	box := m.boxes[m.box].ID
	position := m.position
	return func() tea.Msg {
		emails, q, err := m.client.ListEmails(m.ctx, map[string]any{"inMailbox": box}, position, 50)
		return emailsMsg{emails, q.Total, err}
	}
}
func tick() tea.Cmd { return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(20, msg.Width)
		m.height = max(8, msg.Height)
		m.viewport.SetWidth(m.width - 4)
		m.viewport.SetHeight(m.height - 7)
	case boxesMsg:
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.boxes = msg.boxes
		for i, b := range m.boxes {
			if b.Role == "inbox" {
				m.box = i
				break
			}
		}
		m.loading = true
		return m, tea.Batch(m.fetch(), tick())
	case emailsMsg:
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.emails = msg.emails
		m.total = msg.total
		m.index = max(0, min(m.index, len(m.emails)-1))
		m.status = ""
	case readMsg:
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.reading = true
		e := msg.email
		text := fmt.Sprintf("%s\nFrom: %s\nTo: %s\n\n%s", e.Subject, jmap.FormatAddresses(e.From), jmap.FormatAddresses(e.To), e.Body())
		for _, p := range e.Attachments {
			text += fmt.Sprintf("\nAttachment: %s (%d bytes)", p.Name, p.Size)
		}
		m.viewport.SoftWrap = true
		m.viewport.SetWidth(m.width - 4)
		m.viewport.SetHeight(m.height - 7)
		m.viewport.SetContent(terminal.Sanitize(text))
		m.viewport.GotoTop()
	case mutationMsg:
		m.loading = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.reading = false
		m.loading = true
		return m, m.fetch()
	case tickMsg:
		if !m.loading && !m.reading {
			m.loading = true
			return m, tea.Batch(m.fetch(), tick())
		}
		return m, tick()
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" || key == "q" {
			return m, tea.Quit
		}
		if key == "esc" || key == "backspace" {
			m.reading = false
			return m, nil
		}
		if m.loading {
			return m, nil
		}
		if m.reading {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		switch key {
		case "j", "down":
			m.index = min(m.index+1, max(0, len(m.emails)-1))
		case "k", "up":
			m.index = max(0, m.index-1)
		case "tab", "shift+tab":
			if len(m.boxes) > 0 {
				delta := 1
				if key == "shift+tab" {
					delta = -1
				}
				m.box = (m.box + delta + len(m.boxes)) % len(m.boxes)
				m.index = 0
				m.position = 0
				m.loading = true
				return m, m.fetch()
			}
		case "r":
			m.loading = true
			return m, m.fetch()
		case "n":
			if m.position+len(m.emails) < m.total {
				m.position += 50
				m.index = 0
				m.loading = true
				return m, m.fetch()
			}
		case "p":
			if m.position > 0 {
				m.position = max(0, m.position-50)
				m.index = 0
				m.loading = true
				return m, m.fetch()
			}
		case "enter":
			if len(m.emails) > 0 {
				m.loading = true
				id := m.emails[m.index].ID
				return m, func() tea.Msg {
					emails, err := m.client.Emails(m.ctx, []string{id}, true)
					if err != nil {
						return readMsg{err: err}
					}
					return readMsg{email: emails[0]}
				}
			}
		case "s", "u", "e", "t":
			if len(m.emails) > 0 {
				m.loading = true
				e := m.emails[m.index]
				return m, func() tea.Msg {
					var err error
					switch key {
					case "s":
						var value any = true
						if e.Keywords["$flagged"] {
							value = nil
						}
						err = m.client.UpdateEmails(m.ctx, []string{e.ID}, map[string]any{"keywords/$flagged": value})
					case "u":
						var value any = true
						if e.Keywords["$seen"] {
							value = nil
						}
						err = m.client.UpdateEmails(m.ctx, []string{e.ID}, map[string]any{"keywords/$seen": value})
					case "e":
						err = m.client.Archive(m.ctx, []string{e.ID})
					case "t":
						box, e2 := m.client.RoleMailbox(m.ctx, "trash")
						err = e2
						if err == nil {
							err = m.client.MoveEmails(m.ctx, []string{e.ID}, box.ID, false)
						}
					}
					return mutationMsg{err}
				}
			}
		}
	}
	return m, nil
}
func (m Model) View() tea.View {
	title := "Fastmail"
	if len(m.boxes) > 0 {
		title += "  /  " + terminal.SanitizeLine(m.boxes[m.box].Name)
	}
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(title)
	var content string
	if m.reading {
		content = m.viewport.View()
	} else {
		rows := []string{}
		available := max(1, m.height-8)
		offset := max(0, m.index-available+1)
		for i := offset; i < len(m.emails) && i < offset+available; i++ {
			e := m.emails[i]
			prefix := "  "
			if i == m.index {
				prefix = "> "
			}
			unread := " "
			if !e.Keywords["$seen"] {
				unread = "*"
			}
			sender := ""
			if len(e.From) > 0 {
				sender = e.From[0].Email
			}
			line := fmt.Sprintf("%s%s %-24.24s %s", prefix, unread, terminal.SanitizeLine(sender), terminal.SanitizeLine(e.Subject))
			line = lipgloss.NewStyle().MaxWidth(m.width - 4).Render(line)
			if i == m.index {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
			rows = append(rows, line)
		}
		if len(rows) == 0 {
			rows = append(rows, "No messages")
		}
		content = strings.Join(rows, "\n")
	}
	status := terminal.SanitizeLine(m.status)
	if m.loading {
		status = "Loading…"
	} else if status == "" {
		status = fmt.Sprintf("%d–%d of %d", min(m.position+1, m.total), m.position+len(m.emails), m.total)
	}
	help := "↑↓ select · Enter read · Tab folders · n/p pages\nu read/unread · s star · e archive · t trash · r refresh · q quit"
	if m.reading {
		help = "↑↓ / PgUp / PgDown scroll · Esc back · q quit"
	}
	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(head + "\n\n" + content + "\n\n" + status + "\n" + help))
	view.AltScreen = true
	view.WindowTitle = "Fastmail CLI"
	return view
}
