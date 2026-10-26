package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/output"
)

func (a *app) watchCommand() *cobra.Command {
	var since string
	var interval time.Duration
	var once bool
	cmd := &cobra.Command{Use: "watch", Short: "Stream account-wide email changes as newline-delimited JSON", Long: "Poll Email/changes and emit created, updated, destroyed and checkpoint records.\nStarts at the current state unless --since is supplied. Changes include drafts\nand sent messages. Persist checkpoint.state for restart; delivery is at least once.\nAn expired server cursor is an error; restarting without --since establishes a new baseline.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if a.jq != "" || a.quiet || a.ids || a.count || a.markdown || a.styled {
			return apierr.ErrUsage("watch always writes NDJSON; incompatible output flags were supplied")
		}
		if interval < time.Second {
			return apierr.ErrUsage("--interval must be at least 1s")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		state := since
		emit := func(kind, id, state string) error {
			data := map[string]string{"event": kind}
			if id != "" {
				data["id"] = id
			}
			if state != "" {
				data["state"] = state
			}
			b, marshalErr := output.MarshalJSON(data)
			if marshalErr != nil {
				return marshalErr
			}
			_, writeErr := fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return writeErr
		}
		if state == "" {
			state, err = c.EmailState(cmd.Context())
			if err != nil {
				return err
			}
			if state == "" {
				return apierr.ErrAPI(0, "server returned an empty email state")
			}
		}
		if err = emit("ready", "", state); err != nil {
			return err
		}
		for {
			changes, err := c.Changes(cmd.Context(), state)
			if err != nil {
				if cmd.Context().Err() != nil {
					return cmd.Context().Err()
				}
				return err
			}
			if changes.NewState == "" || changes.HasMoreChanges && changes.NewState == state {
				return apierr.ErrAPI(0, "server returned an invalid changes cursor")
			}
			for _, group := range []struct {
				kind string
				ids  []string
			}{{"created", changes.Created}, {"updated", changes.Updated}, {"destroyed", changes.Destroyed}} {
				for _, id := range group.ids {
					if err = emit(group.kind, id, ""); err != nil {
						return err
					}
				}
			}
			state = changes.NewState
			if err = emit("checkpoint", "", state); err != nil {
				return err
			}
			if changes.HasMoreChanges {
				continue
			}
			if once {
				return nil
			}
			timer := time.NewTimer(interval)
			select {
			case <-cmd.Context().Done():
				timer.Stop()
				return cmd.Context().Err()
			case <-timer.C:
			}
		}
	}}
	cmd.Flags().StringVar(&since, "since", "", "Resume from an opaque Email state")
	cmd.Flags().DurationVar(&interval, "interval", 15*time.Second, "Poll interval (minimum 1s)")
	cmd.Flags().BoolVar(&once, "once", false, "Drain pending changes and exit")
	return cmd
}
