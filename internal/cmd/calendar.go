package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/auth"
	"github.com/JangoCG/fastmail-cli/internal/dav"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

func checkDAV(ctx context.Context, endpoint, username, password string) error {
	c, err := dav.New(endpoint, username, password)
	if err != nil {
		return err
	}
	_, err = c.Calendars(ctx)
	return err
}
func (a *app) calendarAPI() (*dav.Client, error) {
	username := os.Getenv("FASTMAIL_DAV_USERNAME")
	if username == "" {
		username = a.cfg.DAVUsername
	}
	password := os.Getenv("FASTMAIL_DAV_PASSWORD")
	if password == "" {
		var err error
		password, err = (auth.Store{Endpoint: a.cfg.DAVURL}).Get("dav")
		if err != nil {
			return nil, err
		}
	}
	if username == "" || password == "" {
		return nil, &apierr.Error{Code: apierr.CodeAuth, Message: "CalDAV requires a Fastmail username and app password", Hint: "Run: fastmail auth caldav --username your-login (app password on stdin)"}
	}
	return dav.New(a.cfg.DAVURL, username, password)
}
func (a *app) calendarCommand() *cobra.Command {
	p := &cobra.Command{Use: "calendar", Short: "Inspect Fastmail calendars via CalDAV"}
	p.AddCommand(&cobra.Command{Use: "list", Short: "List calendar names and paths", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.calendarAPI()
		if err != nil {
			return err
		}
		calendars, err := c.Calendars(cmd.Context())
		if err != nil {
			return err
		}
		rows := []map[string]any{}
		var lines []string
		for _, cal := range calendars {
			rows = append(rows, map[string]any{"id": cal.Path, "name": cal.Name, "description": cal.Description, "components": cal.SupportedComponentSet})
			lines = append(lines, cal.Path+"\t"+cal.Name)
		}
		return a.result(cmd, rows, strings.Join(lines, "\n"))
	}})
	return p
}
func (a *app) eventCommand() *cobra.Command {
	p := &cobra.Command{Use: "event", Short: "Read, create and edit calendar events"}
	var calendar, after, before string
	list := &cobra.Command{Use: "list", Short: "List event resources overlapping a period (recurring series are not expanded)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		start := time.Now().UTC()
		end := start.AddDate(0, 1, 0)
		var err error
		if after != "" {
			s, e := jmap.Date(after)
			if e != nil {
				return e
			}
			start, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return err
			}
			if before == "" {
				end = start.AddDate(0, 1, 0)
			}
		}
		if before != "" {
			s, e := jmap.Date(before)
			if e != nil {
				return e
			}
			end, err = time.Parse(time.RFC3339, s)
			if err != nil {
				return err
			}
		}
		if !end.After(start) {
			return apierr.ErrUsage("--before must follow --after")
		}
		c, err := a.calendarAPI()
		if err != nil {
			return err
		}
		events, err := c.List(cmd.Context(), calendar, start, end)
		if err != nil {
			return err
		}
		var lines []string
		for _, e := range events {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s", e.ID, e.Start, e.Summary))
		}
		return a.result(cmd, events, strings.Join(lines, "\n"))
	}}
	list.Flags().StringVar(&calendar, "calendar", "", "Calendar path or unambiguous name")
	list.Flags().StringVar(&after, "after", "", "Period start (default: now)")
	list.Flags().StringVar(&before, "before", "", "Exclusive period end (default: one month later)")
	p.AddCommand(list)
	p.AddCommand(&cobra.Command{Use: "read PATH", Short: "Read the full event resource including iCalendar", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.calendarAPI()
		if err != nil {
			return err
		}
		obj, err := c.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return a.writer.OK(dav.Events(obj))
	}})
	p.AddCommand(a.eventWriteCommand(false), a.eventWriteCommand(true))
	var yes, series bool
	del := &cobra.Command{Use: "delete PATH", Short: "Delete an event resource (a recurring resource is an entire series)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return apierr.ErrUsage("event deletion requires --yes")
		}
		c, err := a.calendarAPI()
		if err != nil {
			return err
		}
		obj, err := c.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		for _, e := range dav.Events(obj) {
			if (e.Recurring || e.RecurrenceID != "") && !series {
				return apierr.ErrUsage("deleting a recurring resource requires --series")
			}
		}
		if err = c.Delete(cmd.Context(), args[0], obj.ETag); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Event deleted")
	}}
	del.Flags().BoolVar(&yes, "yes", false, "Confirm deletion")
	del.Flags().BoolVar(&series, "series", false, "Delete the entire recurring event resource")
	p.AddCommand(del)
	return p
}
func (a *app) eventWriteCommand(update bool) *cobra.Command {
	var calendar, start, end, description, location, summary string
	var allDay bool
	use, short := "add TITLE", "Create a calendar event"
	if update {
		use, short = "update PATH", "Edit a nonrecurring event without discarding other iCalendar fields"
	}
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.calendarAPI()
		if err != nil {
			return err
		}
		parseTimes := func() (time.Time, time.Time, error) {
			format := time.RFC3339
			if allDay {
				format = "2006-01-02"
			}
			s, parseErr := time.Parse(format, start)
			if parseErr != nil {
				return s, s, apierr.ErrUsage("invalid --start; use RFC3339 with timezone, or YYYY-MM-DD with --all-day")
			}
			e, parseErr := time.Parse(format, end)
			if parseErr != nil || !e.After(s) {
				return s, e, apierr.ErrUsage("--end must follow --start and use the same date format")
			}
			return s, e, nil
		}
		if !update {
			s, e, timeErr := parseTimes()
			if timeErr != nil {
				return timeErr
			}
			cal, resolveErr := c.Resolve(cmd.Context(), calendar)
			if resolveErr != nil {
				return resolveErr
			}
			path := strings.TrimRight(cal.Path, "/") + "/" + jmap.UUID() + ".ics"
			if err = c.Put(cmd.Context(), path, dav.NewEvent(args[0], description, location, s, e, allDay), "", true); err != nil {
				return err
			}
			return a.mutation(cmd, map[string]string{"id": path}, "Event created: "+path)
		}
		obj, err := c.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		events := dav.Events(obj)
		if len(events) != 1 || events[0].Recurring || events[0].RecurrenceID != "" {
			return apierr.ErrUsage("recurring or multi-event resources must be edited in Fastmail")
		}
		if obj.Data == nil {
			return apierr.ErrAPI(0, "calendar resource has no data")
		}
		var event *ical.Component
		for _, co := range obj.Data.Children {
			if co.Name == ical.CompEvent {
				event = co
				break
			}
		}
		if event == nil {
			return apierr.ErrUsage("resource has no event")
		}
		changed := false
		for flag, prop := range map[string]string{"title": ical.PropSummary, "description": ical.PropDescription, "location": ical.PropLocation} {
			if cmd.Flags().Changed(flag) {
				value := summary
				if flag == "description" {
					value = description
				}
				if flag == "location" {
					value = location
				}
				event.Props.SetText(prop, value)
				changed = true
			}
		}
		if cmd.Flags().Changed("start") || cmd.Flags().Changed("end") || cmd.Flags().Changed("all-day") {
			s, e, timeErr := parseTimes()
			if timeErr != nil {
				return timeErr
			}
			dav.SetTimes(event, s, e, allDay)
			changed = true
		}
		if !changed {
			return apierr.ErrUsage("provide --title, --description, --location, or both --start and --end")
		}
		event.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
		if err = c.Put(cmd.Context(), args[0], obj.Data, obj.ETag, false); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Event updated")
	}}
	cmd.Flags().StringVar(&calendar, "calendar", "", "Calendar path or name")
	cmd.Flags().StringVar(&start, "start", "", "Start RFC3339 time, or date with --all-day")
	cmd.Flags().StringVar(&end, "end", "", "Exclusive end time or date")
	cmd.Flags().BoolVar(&allDay, "all-day", false, "Use whole dates (end date is exclusive)")
	cmd.Flags().StringVar(&description, "description", "", "Event description")
	cmd.Flags().StringVar(&location, "location", "", "Event location")
	if update {
		cmd.Flags().StringVar(&summary, "title", "", "Replacement title")
	}
	return cmd
}
