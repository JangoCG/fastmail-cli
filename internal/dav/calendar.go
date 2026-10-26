// Package dav implements Fastmail calendar access through CalDAV.
package dav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/version"
)

type Client struct {
	DAV                *caldav.Client
	http               *http.Client
	base               *url.URL
	username, password string
}

func New(endpoint, username, password string) (*Client, error) {
	if err := jmap.ValidateURL(endpoint); err != nil {
		return nil, err
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	c := &Client{base: base, username: username, password: password, http: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	c.DAV, err = caldav.NewClient(c, endpoint)
	return c, err
}

// Do confines Basic credentials and server-supplied DAV paths to one origin.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != c.base.Scheme || req.URL.Host != c.base.Host || req.URL.User != nil {
		return nil, apierr.ErrUsage("CalDAV URL must remain on the configured server")
	}
	req.Header.Set("User-Agent", version.UserAgent())
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		_ = resp.Body.Close()
		return nil, &apierr.Error{Code: apierr.CodeAuth, Message: "CalDAV authentication failed", Hint: "Run: fastmail auth caldav --username your-login (app password on stdin)"}
	}
	return resp, nil
}
func (c *Client) Calendars(ctx context.Context) ([]caldav.Calendar, error) {
	principal, err := c.DAV.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, err
	}
	home, err := c.DAV.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return nil, err
	}
	return c.DAV.FindCalendars(ctx, home)
}
func (c *Client) Resolve(ctx context.Context, value string) (caldav.Calendar, error) {
	calendars, err := c.Calendars(ctx)
	if err != nil {
		return caldav.Calendar{}, err
	}
	var matches []caldav.Calendar
	for _, cal := range calendars {
		if cal.Path == value {
			return cal, nil
		}
		if strings.EqualFold(cal.Name, value) {
			matches = append(matches, cal)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if value == "" && len(calendars) == 1 {
		return calendars[0], nil
	}
	return caldav.Calendar{}, apierr.ErrUsage("choose --calendar using a path from 'fastmail calendar list'")
}

type Event struct {
	ID           string `json:"id"`
	ETag         string `json:"etag"`
	UID          string `json:"uid"`
	Summary      string `json:"summary"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Location     string `json:"location,omitempty"`
	Description  string `json:"description,omitempty"`
	RecurrenceID string `json:"recurrenceId,omitempty"`
	Recurring    bool   `json:"recurring"`
	ICS          string `json:"ics,omitempty"`
}

func Events(obj *caldav.CalendarObject) []Event {
	if obj.Data == nil {
		return nil
	}
	var buf bytes.Buffer
	_ = ical.NewEncoder(&buf).Encode(obj.Data)
	var out []Event
	for _, co := range obj.Data.Children {
		if co.Name != ical.CompEvent {
			continue
		}
		text := func(k string) string { s, _ := co.Props.Text(k); return s }
		value := func(k string) string {
			if p := co.Props.Get(k); p != nil {
				return p.Value
			}
			return ""
		}
		out = append(out, Event{ID: obj.Path, ETag: obj.ETag, UID: text(ical.PropUID), Summary: text(ical.PropSummary), Start: value(ical.PropDateTimeStart), End: value(ical.PropDateTimeEnd), Location: text(ical.PropLocation), Description: text(ical.PropDescription), Recurring: co.Props.Get(ical.PropRecurrenceRule) != nil, RecurrenceID: value(ical.PropRecurrenceID), ICS: buf.String()})
	}
	return out
}
func (c *Client) List(ctx context.Context, calendar string, start, end time.Time) ([]Event, error) {
	cal, err := c.Resolve(ctx, calendar)
	if err != nil {
		return nil, err
	}
	objects, err := c.DAV.QueryCalendar(ctx, cal.Path, &caldav.CalendarQuery{CompRequest: caldav.CalendarCompRequest{Name: ical.CompCalendar, AllProps: true, AllComps: true}, CompFilter: caldav.CompFilter{Name: ical.CompCalendar, Comps: []caldav.CompFilter{{Name: ical.CompEvent, Start: start, End: end}}}})
	if err != nil {
		return nil, err
	}
	result := []Event{}
	for _, obj := range objects {
		events := Events(&obj)
		for i := range events {
			events[i].ICS = ""
		}
		result = append(result, events...)
	}
	return result, nil
}
func (c *Client) Get(ctx context.Context, path string) (*caldav.CalendarObject, error) {
	if err := c.eventPath(path); err != nil {
		return nil, err
	}
	return c.DAV.GetCalendarObject(ctx, path)
}
func (c *Client) eventPath(path string) error {
	u, err := url.Parse(path)
	if err != nil {
		return err
	}
	u = c.base.ResolveReference(u)
	if u.Scheme != c.base.Scheme || u.Host != c.base.Host || !strings.HasSuffix(u.Path, ".ics") || u.RawQuery != "" || u.Fragment != "" || strings.Contains(u.Path, "/../") {
		return apierr.ErrUsage("event ID must be an .ics resource on the configured CalDAV server")
	}
	return nil
}
func (c *Client) Put(ctx context.Context, path string, cal *ical.Calendar, etag string, create bool) error {
	if err := c.eventPath(path); err != nil {
		return err
	}
	var body bytes.Buffer
	if err := ical.NewEncoder(&body).Encode(cal); err != nil {
		return err
	}
	return c.mutate(ctx, http.MethodPut, path, &body, etag, create)
}
func (c *Client) Delete(ctx context.Context, path, etag string) error {
	if err := c.eventPath(path); err != nil {
		return err
	}
	return c.mutate(ctx, http.MethodDelete, path, nil, etag, false)
}
func (c *Client) mutate(ctx context.Context, method, path string, body io.Reader, etag string, create bool) error {
	u, err := url.Parse(path)
	if err != nil {
		return err
	}
	u = c.base.ResolveReference(u)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	if create {
		req.Header.Set("If-None-Match", "*")
	} else {
		if etag == "" {
			return apierr.ErrConflict(0, "calendar server returned no ETag; refusing an unguarded write", "", nil)
		}
		if !strings.HasPrefix(etag, `"`) && !strings.HasPrefix(etag, `W/"`) {
			etag = strconv.Quote(etag)
		}
		req.Header.Set("If-Match", etag)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 412 {
		return apierr.ErrConflict(412, "calendar event changed; read it again before editing", "", nil)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apierr.ErrAPI(resp.StatusCode, fmt.Sprintf("CalDAV %s returned HTTP %d", method, resp.StatusCode))
	}
	return nil
}
func NewEvent(summary, description, location string, start, end time.Time, allDay bool) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Fastmail CLI//EN")
	event := ical.NewComponent(ical.CompEvent)
	event.Props.SetText(ical.PropUID, jmap.UUID())
	event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	event.Props.SetText(ical.PropSummary, summary)
	event.Props.SetText(ical.PropDescription, description)
	event.Props.SetText(ical.PropLocation, location)
	SetTimes(event, start, end, allDay)
	cal.Children = append(cal.Children, event)
	return cal
}
func SetTimes(event *ical.Component, start, end time.Time, allDay bool) {
	event.Props.Del(ical.PropDuration)
	if allDay {
		event.Props.SetDate(ical.PropDateTimeStart, start)
		event.Props.SetDate(ical.PropDateTimeEnd, end)
	} else {
		event.Props.SetDateTime(ical.PropDateTimeStart, start.UTC())
		event.Props.SetDateTime(ical.PropDateTimeEnd, end.UTC())
	}
}
