package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

func (a *app) maskedCommand() *cobra.Command {
	p := &cobra.Command{Use: "masked-email", Short: "Manage Fastmail Masked Email addresses"}
	p.AddCommand(&cobra.Command{Use: "list", Short: "List masked addresses", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		var r jmap.GetResult[map[string]any]
		if err = c.Call(cmd.Context(), jmap.Masked, "MaskedEmail/get", nil, &r); err != nil {
			return err
		}
		var lines []string
		for _, v := range r.List {
			lines = append(lines, fmt.Sprintf("%v\t%v\t%v\t%v", v["id"], v["email"], v["state"], v["description"]))
		}
		return a.result(cmd, r.List, strings.Join(lines, "\n"))
	}})
	var domain, description, prefix string
	create := &cobra.Command{Use: "create", Short: "Create an enabled masked address", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if domain != "" {
			u, err := url.Parse(domain)
			if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
				return apierr.ErrUsage("--for-domain must be an origin such as https://example.com")
			}
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		obj := map[string]any{"state": "enabled", "forDomain": domain, "description": description}
		if prefix != "" {
			obj["emailPrefix"] = prefix
		}
		var r jmap.SetResult
		if err = c.Call(cmd.Context(), jmap.Masked, "MaskedEmail/set", map[string]any{"create": map[string]any{"masked": obj}}, &r); err != nil {
			return err
		}
		return a.mutation(cmd, r, "Masked Email created")
	}}
	create.Flags().StringVar(&domain, "for-domain", "", "Website origin, e.g. https://example.com")
	create.Flags().StringVar(&description, "description", "", "User-supplied description")
	create.Flags().StringVar(&prefix, "prefix", "", "Email prefix (a-z, 0-9, underscore)")
	p.AddCommand(create)
	var state, desc string
	update := &cobra.Command{Use: "update ID", Short: "Change state or description", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		patch := map[string]any{}
		if cmd.Flags().Changed("state") {
			switch state {
			case "enabled", "disabled", "deleted":
				patch["state"] = state
			default:
				return apierr.ErrUsage("state must be enabled, disabled or deleted")
			}
		}
		if cmd.Flags().Changed("description") {
			patch["description"] = desc
		}
		if len(patch) == 0 {
			return apierr.ErrUsage("provide --state or --description")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		if err = c.Call(cmd.Context(), jmap.Masked, "MaskedEmail/set", map[string]any{"update": map[string]any{args[0]: patch}}, nil); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Masked Email updated")
	}}
	update.Flags().StringVar(&state, "state", "", "enabled, disabled or deleted")
	update.Flags().StringVar(&desc, "description", "", "Description")
	p.AddCommand(update)
	return p
}
func (a *app) vacationCommand() *cobra.Command {
	p := &cobra.Command{Use: "vacation", Short: "Manage automatic vacation replies"}
	p.AddCommand(&cobra.Command{Use: "get", Short: "Read vacation response", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		var r jmap.GetResult[map[string]any]
		if err = c.Call(cmd.Context(), jmap.Vacation, "VacationResponse/get", nil, &r); err != nil {
			return err
		}
		return a.writer.OK(r.List)
	}})
	var subject, body, start, end string
	set := &cobra.Command{Use: "set", Short: "Enable a vacation reply", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if body == "" {
			return apierr.ErrUsage("--message is required")
		}
		patch := map[string]any{"isEnabled": true, "subject": subject, "textBody": body, "htmlBody": nil, "fromDate": nil, "toDate": nil}
		for k, v := range map[string]string{"fromDate": start, "toDate": end} {
			if v != "" {
				d, err := jmap.Date(v)
				if err != nil {
					return err
				}
				patch[k] = d
			}
		}
		fromDate, _ := patch["fromDate"].(string)
		toDate, _ := patch["toDate"].(string)
		if fromDate != "" && toDate != "" && fromDate >= toDate {
			return apierr.ErrUsage("--until must follow --from")
		}
		return a.setVacation(cmd, patch)
	}}
	set.Flags().StringVar(&subject, "subject", "", "Reply subject")
	set.Flags().StringVarP(&body, "message", "m", "", "Reply text (required)")
	set.Flags().StringVar(&start, "from", "", "Start date or RFC3339 time")
	set.Flags().StringVar(&end, "until", "", "Exclusive end date or RFC3339 time")
	p.AddCommand(set)
	p.AddCommand(&cobra.Command{Use: "disable", Short: "Disable the vacation reply", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return a.setVacation(cmd, map[string]any{"isEnabled": false})
	}})
	return p
}
func (a *app) setVacation(cmd *cobra.Command, patch map[string]any) error {
	c, err := a.api(cmd.Context())
	if err != nil {
		return err
	}
	if err = c.Call(cmd.Context(), jmap.Vacation, "VacationResponse/set", map[string]any{"update": map[string]any{"singleton": patch}}, nil); err != nil {
		return err
	}
	return a.mutation(cmd, patch, "Vacation response updated")
}
