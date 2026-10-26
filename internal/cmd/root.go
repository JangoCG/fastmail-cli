package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/auth"
	"github.com/JangoCG/fastmail-cli/internal/config"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/output"
	"github.com/JangoCG/fastmail-cli/internal/terminal"
	"github.com/JangoCG/fastmail-cli/internal/version"
)

type app struct {
	cfg                                       config.Config
	client                                    *jmap.Client
	writer                                    *output.Writer
	json, quiet, ids, count, markdown, styled bool
	jq, account, sessionURL                   string
}

func NewRoot(ctx context.Context) *cobra.Command {
	a := &app{}
	root := &cobra.Command{Use: "fastmail", Short: "Fastmail mail, contacts and calendars from your terminal", Version: version.Version, SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs}
	root.SetContext(ctx)
	root.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	flags := root.PersistentFlags()
	flags.BoolVar(&a.json, "json", false, "Write a JSON response envelope")
	flags.BoolVar(&a.quiet, "quiet", false, "Write only JSON data")
	flags.BoolVar(&a.ids, "ids-only", false, "Print one ID per line (lists only)")
	flags.BoolVar(&a.count, "count", false, "Print number of returned items")
	flags.BoolVar(&a.markdown, "markdown", false, "Write Markdown")
	flags.BoolVar(&a.styled, "styled", false, "Write readable terminal output")
	flags.StringVar(&a.jq, "jq", "", "Filter the response with jq")
	flags.StringVar(&a.account, "account", os.Getenv("FASTMAIL_ACCOUNT"), "JMAP account ID")
	flags.StringVar(&a.sessionURL, "session-url", os.Getenv("FASTMAIL_SESSION_URL"), "JMAP session URL")
	root.MarkFlagsMutuallyExclusive("json", "quiet", "ids-only", "count", "markdown", "styled")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		a.writer = output.New(output.Options{Format: output.FormatFromFlags(a.json, a.quiet, a.ids, a.count, a.markdown, a.styled, false), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(), JQFilter: a.jq})
		if err := output.ValidateJQFilter(a.jq); err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		a.cfg = cfg
		if a.sessionURL != "" {
			a.cfg.SessionURL = a.sessionURL
		}
		if a.account != "" {
			a.cfg.AccountID = a.account
		}
		if err = jmap.ValidateURL(a.cfg.SessionURL); err != nil {
			return err
		}
		return nil
	}
	root.AddCommand(a.authCommand(), a.accountsCommand(), a.configCommand(), a.doctorCommand(), a.mailboxCommand(), a.emailCommand(), a.searchCommand(), a.threadCommand(), a.composeCommand("compose"), a.replyCommand(false), a.replyCommand(true), a.draftCommand(), a.attachmentCommand(), a.identityCommand(), a.contactsCommand(), a.addressbookCommand(), a.maskedCommand(), a.vacationCommand(), a.calendarCommand(), a.eventCommand(), a.watchCommand(), a.tuiCommand(), a.mcpCommand(), a.setupCommand()) //nolint:contextcheck // Cobra constructors defer handlers; ExecuteContext supplies cmd.Context() when they run.
	root.AddCommand(&cobra.Command{Use: "version", Short: "Show build version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return a.result(cmd, map[string]string{"version": version.Version, "commit": version.Commit, "date": version.Date}, version.Full())
	}})
	completion := &cobra.Command{Use: "completion bash|zsh|fish|powershell", Short: "Generate shell completion", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		default:
			return apierr.ErrUsage("unknown shell")
		}
	}}
	root.AddCommand(completion)
	return root
}
func (a *app) api(ctx context.Context) (*jmap.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	token := strings.TrimSpace(os.Getenv("FASTMAIL_TOKEN"))
	if token == "" {
		var err error
		token, err = (auth.Store{Endpoint: a.cfg.SessionURL}).Get("token")
		if err != nil {
			return nil, err
		}
	}
	if token == "" {
		return nil, apierr.ErrAuth("Fastmail API token is missing")
	}
	c := jmap.New(token, a.cfg.SessionURL, a.cfg.AccountID)
	if _, err := c.Discover(ctx); err != nil {
		return nil, err
	}
	a.client = c
	return c, nil
}
func (a *app) result(cmd *cobra.Command, data any, human string, opts ...output.ResponseOption) error {
	if a.writer.IsStyled() && a.jq == "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), terminal.Sanitize(human))
		return err
	}
	return a.writer.OK(data, opts...)
}
func (a *app) mutation(cmd *cobra.Command, data any, message string) error {
	return a.result(cmd, data, message, output.WithSummary(message))
}
func Execute(ctx context.Context) int {
	root := NewRoot(ctx)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	format := output.FormatAuto
	// Parse errors can occur before pre-run; still honour explicit JSON/styled output.
	if f := root.PersistentFlags().Lookup("json"); f != nil && f.Value.String() == "true" {
		format = output.FormatJSON
	}
	if f := root.PersistentFlags().Lookup("styled"); f != nil && f.Value.String() == "true" {
		format = output.FormatStyled
	}
	output.New(output.Options{Format: format}).Err(err)
	return output.ExitCodeFor(err)
}
func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, (8<<20)+1))
	if len(data) > 8<<20 {
		return nil, apierr.ErrUsage("input exceeds 8 MiB")
	}
	return data, err
}
