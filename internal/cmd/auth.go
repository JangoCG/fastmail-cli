package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/auth"
	"github.com/JangoCG/fastmail-cli/internal/config"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
)

func (a *app) authCommand() *cobra.Command {
	parent := &cobra.Command{Use: "auth", Short: "Manage Fastmail API and CalDAV credentials"}
	var stdin bool
	login := &cobra.Command{Use: "login", Short: "Validate and store an API token", Long: "Create a JMAP token at https://app.fastmail.com/settings/security/tokens.\nUse --token-stdin, FASTMAIL_TOKEN, or the hidden interactive prompt.\nTokens are stored in the system keyring, with a private file fallback.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		token := strings.TrimSpace(os.Getenv("FASTMAIL_TOKEN"))
		if stdin {
			data, err := readLimited(cmd.InOrStdin())
			if err != nil {
				return err
			}
			token = strings.TrimSpace(string(data))
		} else if token == "" {
			f, ok := cmd.InOrStdin().(*os.File)
			if !ok || !term.IsTerminal(int(f.Fd())) {
				return apierr.ErrUsage("use --token-stdin or FASTMAIL_TOKEN for noninteractive login")
			}
			fmt.Fprint(cmd.ErrOrStderr(), "Fastmail API token: ")
			data, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			token = strings.TrimSpace(string(data))
		}
		if token == "" {
			return apierr.ErrUsage("token is empty")
		}
		c := jmap.New(token, a.cfg.SessionURL, a.cfg.AccountID)
		session, err := c.Discover(cmd.Context())
		if err != nil {
			return err
		}
		if _, err = c.Account(cmd.Context(), jmap.Mail); err != nil {
			return err
		}
		storage, err := (auth.Store{Endpoint: a.cfg.SessionURL}).Set("token", token)
		if err != nil {
			return err
		}
		if err = config.Save(a.cfg); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]any{"username": session.Username, "storage": storage}, "Signed in as "+session.Username+" ("+storage+")")
	}}
	login.Flags().BoolVar(&stdin, "token-stdin", false, "Read API token from standard input")
	parent.AddCommand(login)
	parent.AddCommand(&cobra.Command{Use: "status", Short: "Check the token with Fastmail", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		s, err := c.Discover(cmd.Context())
		if err != nil {
			return err
		}
		return a.result(cmd, map[string]any{"username": s.Username, "accounts": s.Accounts, "session_url": a.cfg.SessionURL}, "Signed in as "+s.Username)
	}})
	parent.AddCommand(&cobra.Command{Use: "logout", Short: "Remove locally stored credentials (does not revoke tokens)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s := auth.Store{Endpoint: a.cfg.SessionURL}
		if err := s.Delete("token"); err != nil {
			return err
		}
		if err := (auth.Store{Endpoint: a.cfg.DAVURL}).Delete("dav"); err != nil {
			return err
		}
		return a.mutation(cmd, nil, "Stored credentials removed. Revoke the token in Fastmail settings if needed; environment credentials remain active.")
	}})
	var username string
	dav := &cobra.Command{Use: "caldav", Short: "Store a CalDAV app password from stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if username == "" {
			return apierr.ErrUsage("--username is required")
		}
		data, err := readLimited(cmd.InOrStdin())
		if err != nil {
			return err
		}
		password := strings.TrimSpace(string(data))
		if password == "" {
			return apierr.ErrUsage("app password is empty")
		}
		if err = jmap.ValidateURL(a.cfg.DAVURL); err != nil {
			return err
		}
		if err = checkDAV(cmd.Context(), a.cfg.DAVURL, username, password); err != nil {
			return err
		}
		storage, err := (auth.Store{Endpoint: a.cfg.DAVURL}).Set("dav", password)
		if err != nil {
			return err
		}
		a.cfg.DAVUsername = username
		if err = config.Save(a.cfg); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"username": username, "storage": storage}, "CalDAV credentials saved")
	}}
	dav.Flags().StringVar(&username, "username", "", "Fastmail login email address (required)")
	parent.AddCommand(dav)
	return parent
}
func (a *app) accountsCommand() *cobra.Command {
	return &cobra.Command{Use: "accounts", Short: "List accessible JMAP accounts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		s, err := c.Discover(cmd.Context())
		if err != nil {
			return err
		}
		var items []map[string]any
		var lines []string
		for id, account := range s.Accounts {
			items = append(items, map[string]any{"id": id, "name": account.Name, "readOnly": account.IsReadOnly, "capabilities": account.AccountCapabilities})
			lines = append(lines, id+"\t"+account.Name)
		}
		return a.result(cmd, items, strings.Join(lines, "\n"))
	}}
}
func (a *app) identityCommand() *cobra.Command {
	p := &cobra.Command{Use: "identity", Short: "Inspect available sending identities"}
	p.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, Short: "List identities", RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		ids, err := c.Identities(cmd.Context())
		if err != nil {
			return err
		}
		var lines []string
		for _, i := range ids {
			lines = append(lines, fmt.Sprintf("%s\t%s <%s>", i.ID, i.Name, i.Email))
		}
		return a.result(cmd, ids, strings.Join(lines, "\n"))
	}})
	return p
}
func (a *app) configCommand() *cobra.Command {
	p := &cobra.Command{Use: "config", Short: "Manage local preferences (credentials are never printed)"}
	p.AddCommand(&cobra.Command{Use: "show", Args: cobra.NoArgs, Short: "Show effective configuration", RunE: func(cmd *cobra.Command, _ []string) error { return a.writer.OK(a.cfg) }})
	p.AddCommand(&cobra.Command{Use: "set KEY VALUE", Args: cobra.ExactArgs(2), Short: "Set account, identity, session-url, dav-url or dav-username", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		switch args[0] {
		case "account":
			cfg.AccountID = args[1]
		case "identity":
			cfg.Identity = args[1]
		case "session-url":
			if err = jmap.ValidateURL(args[1]); err != nil {
				return err
			}
			cfg.SessionURL = args[1]
		case "dav-url":
			if err = jmap.ValidateURL(args[1]); err != nil {
				return err
			}
			cfg.DAVURL = args[1]
		case "dav-username":
			cfg.DAVUsername = args[1]
		default:
			return apierr.ErrUsage("unknown configuration key")
		}
		if err = config.Save(cfg); err != nil {
			return err
		}
		return a.mutation(cmd, cfg, "Configuration saved")
	}})
	return p
}
func (a *app) doctorCommand() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Check configuration, authentication and JMAP capabilities", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		s, err := c.Discover(cmd.Context())
		if err != nil {
			return err
		}
		_, err = c.Account(cmd.Context(), jmap.Mail)
		if err != nil {
			return err
		}
		return a.result(cmd, map[string]any{"version": "dev", "username": s.Username, "api_url": s.APIURL, "capabilities": s.Capabilities, "caldav_configured": a.cfg.DAVUsername != ""}, "Fastmail JMAP is reachable; authentication and mail capability work.")
	}}
}
