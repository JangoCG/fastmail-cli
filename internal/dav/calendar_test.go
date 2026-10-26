package dav

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
)

const fixtureICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Example//EN\r\nBEGIN:VEVENT\r\nUID:review@example.com\r\nDTSTAMP:20261026T120000Z\r\nDTSTART:20261030T130000Z\r\nDTEND:20261030T140000Z\r\nSUMMARY:Design review\r\nX-EXAMPLE-CUSTOM:preserve me\r\nBEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:Reminder\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func multi(href, props string) string {
	return `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>` + href + `</d:href><d:propstat><d:prop>` + props + `</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`
}
func TestDiscoveryQueryAndConditionalUpdate(t *testing.T) {
	methods := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods[r.Method]++
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice@example.com" || password != "app-password" {
			t.Error("missing app password authentication")
			w.WriteHeader(401)
			return
		}
		if r.Method == "PROPFIND" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(207)
			switch r.URL.Path {
			case "/":
				_, _ = io.WriteString(w, multi("/", `<d:current-user-principal><d:href>/principal/</d:href></d:current-user-principal>`))
			case "/principal/":
				_, _ = io.WriteString(w, multi("/principal/", `<c:calendar-home-set><d:href>/calendars/</d:href></c:calendar-home-set>`))
			case "/calendars/":
				_, _ = io.WriteString(w, multi("/calendars/personal/", `<d:resourcetype><d:collection/><c:calendar/></d:resourcetype><d:displayname>Personal</d:displayname><c:calendar-description>Private</c:calendar-description><c:max-resource-size>1000000</c:max-resource-size><c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set>`))
			default:
				t.Errorf("path=%s", r.URL.Path)
			}
			return
		}
		if r.Method == "REPORT" {
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("time-range")) || !bytes.Contains(body, []byte("20261001T000000Z")) {
				t.Errorf("query=%s", body)
			}
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(fixtureICS))
			w.WriteHeader(207)
			_, _ = io.WriteString(w, multi("/calendars/personal/review.ics", `<d:getetag>"v1"</d:getetag><c:calendar-data>`+escaped.String()+`</c:calendar-data>`))
			return
		}
		if r.Method == "GET" {
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Content-Type", "text/calendar")
			_, _ = io.WriteString(w, fixtureICS)
			return
		}
		if r.Method == "PUT" {
			if r.Header.Get("If-Match") != `"v1"` {
				t.Errorf("invalid If-Match=%q", r.Header.Get("If-Match"))
				w.WriteHeader(412)
				return
			}
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("X-EXAMPLE-CUSTOM:preserve me")) || !bytes.Contains(body, []byte("BEGIN:VALARM")) || !bytes.Contains(body, []byte("SUMMARY:Updated review")) {
				t.Errorf("iCalendar data lost: %s", body)
			}
			w.WriteHeader(204)
			return
		}
		t.Errorf("unexpected %s", r.Method)
		w.WriteHeader(400)
	}))
	defer server.Close()
	c, err := New(server.URL, "alice@example.com", "app-password")
	if err != nil {
		t.Fatal(err)
	}
	calendars, err := c.Calendars(t.Context())
	if err != nil || len(calendars) != 1 || calendars[0].Name != "Personal" {
		t.Fatal(calendars, err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	events, err := c.List(t.Context(), "Personal", start, start.AddDate(0, 1, 0))
	if err != nil || len(events) != 1 || events[0].Summary != "Design review" {
		t.Fatal(events, err)
	}
	obj, err := c.Get(t.Context(), "/calendars/personal/review.ics")
	if err != nil {
		t.Fatal(err)
	}
	obj.Data.Children[0].Props.SetText(ical.PropSummary, "Updated review")
	if err = c.Put(t.Context(), obj.Path, obj.Data, obj.ETag, false); err != nil {
		t.Fatal(err)
	}
	if methods["REPORT"] != 1 || methods["PUT"] != 1 {
		t.Fatal(methods)
	}
}
func TestCalendarWritesNeverOverwriteWithoutPreconditions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && r.Header.Get("If-None-Match") == "*" {
			w.WriteHeader(201)
			return
		}
		if r.Method == "DELETE" && r.Header.Get("If-Match") == `"old"` {
			w.WriteHeader(412)
			return
		}
		t.Errorf("unconditional mutation %s %v", r.Method, r.Header)
		w.WriteHeader(500)
	}))
	defer server.Close()
	c, _ := New(server.URL, "alice@example.com", "app-password")
	now := time.Now()
	cal := NewEvent("Lunch", "Bring notes", "Cafe", now, now.Add(time.Hour), false)
	if err := c.Put(t.Context(), "/cal/new.ics", cal, "", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(t.Context(), "/cal/existing.ics", cal, "", false); apierr.AsError(err).Code != apierr.CodeConflict {
		t.Fatal(err)
	}
	if err := c.Delete(t.Context(), "/cal/existing.ics", "old"); apierr.AsError(err).Code != apierr.CodeConflict {
		t.Fatal(err)
	}
	for _, path := range []string{"https://example.org/calendar/event.ics", "/cal/", "/cal/event.ics?delete=all"} {
		if _, err := c.Get(t.Context(), path); err == nil {
			t.Fatal("unsafe resource accepted", path)
		}
	}
}
func TestAllDayAndTextEscaping(t *testing.T) {
	start := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	cal := NewEvent("Planning\nwith Alice", "First; second, third\\fourth", "Office", start, start.AddDate(0, 0, 1), true)
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "DTSTART;VALUE=DATE:20261026") || !strings.Contains(buf.String(), "DTEND;VALUE=DATE:20261027") {
		t.Fatal(buf.String())
	}
	decoded, err := ical.NewDecoder(&buf).Decode()
	if err != nil {
		t.Fatal(err)
	}
	value, _ := decoded.Children[0].Props.Text(ical.PropSummary)
	if value != "Planning\nwith Alice" {
		t.Fatal(value)
	}
}
func TestCalDAVDoesNotForwardAppPassword(t *testing.T) {
	calls := 0
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer foreign.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", foreign.URL)
		w.WriteHeader(302)
	}))
	defer server.Close()
	c, _ := New(server.URL, "alice@example.com", "app-password")
	if _, err := c.Calendars(t.Context()); err == nil {
		t.Fatal("redirect accepted")
	}
	req, _ := http.NewRequestWithContext(t.Context(), "GET", foreign.URL, nil)
	if resp, err := c.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("cross-origin request accepted")
	}
	if calls != 0 {
		t.Fatalf("credentials sent to foreign server %d times", calls)
	}
}
