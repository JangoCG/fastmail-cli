package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/mcpserver"
	"github.com/JangoCG/fastmail-cli/skills"
)

func (a *app) mcpCommand() *cobra.Command {
	return &cobra.Command{Use: "mcp", Short: "Serve MCP read tools and draft creation on stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		return mcpserver.New(c, a.cfg.Identity).Run(cmd.Context(), &mcp.StdioTransport{})
	}}
}
func (a *app) setupCommand() *cobra.Command {
	var directory string
	cmd := &cobra.Command{Use: "setup codex|claude", Short: "Install the Fastmail skill for a coding agent", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "codex" && args[0] != "claude" {
			return apierr.ErrUsage("choose codex or claude")
		}
		if directory == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			base := filepath.Join(home, "."+args[0])
			if args[0] == "codex" && os.Getenv("CODEX_HOME") != "" {
				base = os.Getenv("CODEX_HOME")
			}
			directory = filepath.Join(base, "skills", "fastmail")
		}
		// Only a directory explicitly owned by this CLI may be refreshed.
		info, err := os.Lstat(directory)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return apierr.ErrUsage("skill destination must be a real directory")
			}
			entries, readErr := os.ReadDir(directory)
			if readErr != nil {
				return readErr
			}
			if len(entries) > 0 {
				if _, err = os.Stat(filepath.Join(directory, ".managed-by-fastmail-cli")); err != nil {
					return apierr.ErrUsage("skill directory is not managed by fastmail-cli; choose --dir")
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(directory, ".managed-by-fastmail-cli"), []byte("fastmail-cli\n"), 0600); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(directory, "SKILL.md"), skills.Fastmail, 0600); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"directory": directory}, fmt.Sprintf("Fastmail skill installed for %s: %s", args[0], directory))
	}}
	cmd.Flags().StringVar(&directory, "dir", "", "Explicit skill destination directory")
	return cmd
}
