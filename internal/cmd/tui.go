package cmd

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/tui"
)

func (a *app) tuiCommand() *cobra.Command {
	return &cobra.Command{Use: "tui", Short: "Open the interactive Fastmail mailbox reader", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return apierr.ErrUsage("tui requires an interactive terminal")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(tui.New(cmd.Context(), c), tea.WithContext(cmd.Context()), tea.WithInput(cmd.InOrStdin()), tea.WithOutput(cmd.OutOrStdout())).Run()
		return err
	}}
}
