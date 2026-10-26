package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/jmap"
	"github.com/JangoCG/fastmail-cli/internal/output"
)

func (a *app) addressbookCommand() *cobra.Command {
	p := &cobra.Command{Use: "addressbook", Short: "Inspect Fastmail address books"}
	p.AddCommand(&cobra.Command{Use: "list", Short: "List address books and rights", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		books, err := c.AddressBooks(cmd.Context())
		if err != nil {
			return err
		}
		var lines []string
		for _, b := range books {
			lines = append(lines, b.ID+"\t"+b.Name)
		}
		return a.result(cmd, books, strings.Join(lines, "\n"))
	}})
	return p
}
func (a *app) contactsCommand() *cobra.Command {
	p := &cobra.Command{Use: "contact", Aliases: []string{"contacts"}, Short: "Read and manage contacts through JMAP"}
	for _, use := range []string{"list [TEXT...]", "search TEXT..."} {
		var limit, position int
		var all bool
		list := &cobra.Command{Use: use, Short: "List or search contact cards", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 1000 || position < 0 {
				return apierr.ErrUsage("invalid --limit or --position")
			}
			c, err := a.api(cmd.Context())
			if err != nil {
				return err
			}
			cards := []jmap.ContactCard{}
			var total int
			for {
				page, q, err := c.ListContacts(cmd.Context(), strings.Join(args, " "), position, limit)
				if err != nil {
					return err
				}
				cards = append(cards, page...)
				total = q.Total
				position += len(page)
				if !all || len(page) == 0 || position >= total {
					break
				}
			}
			var lines []string
			for _, card := range cards {
				lines = append(lines, contactLine(card))
			}
			return a.result(cmd, cards, strings.Join(lines, "\n"), output.WithMeta("total", total), output.WithMeta("next_position", position))
		}}
		list.Flags().IntVar(&limit, "limit", 50, "Page size (1..1000)")
		list.Flags().IntVar(&position, "position", 0, "Start position")
		list.Flags().BoolVar(&all, "all", false, "Fetch every page")
		p.AddCommand(list)
	}
	p.AddCommand(&cobra.Command{Use: "read ID", Short: "Read a complete JSContact card", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		card, err := c.Contact(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return a.writer.OK(card)
	}})
	var name, email, phone, book string
	add := &cobra.Command{Use: "add", Short: "Create a contact", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(name) == "" {
			return apierr.ErrUsage("--name is required")
		}
		if email != "" {
			addresses, err := jmap.ParseAddresses(email)
			if err != nil {
				return err
			}
			if len(addresses) != 1 {
				return apierr.ErrUsage("--email must contain one address")
			}
			email = addresses[0].Email
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		id, err := c.CreateContact(cmd.Context(), name, email, phone, book)
		if err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": id}, "Contact created: "+id)
	}}
	add.Flags().StringVar(&name, "name", "", "Full name (required)")
	add.Flags().StringVar(&email, "email", "", "Email address")
	add.Flags().StringVar(&phone, "phone", "", "Phone number")
	add.Flags().StringVar(&book, "addressbook", "", "Writable address book ID (default: account default)")
	p.AddCommand(add)
	var newName, newEmail, newPhone string
	update := &cobra.Command{Use: "update ID", Short: "Update name or add/update the CLI email or phone field", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		patch := map[string]any{}
		if cmd.Flags().Changed("name") {
			if strings.TrimSpace(newName) == "" {
				return apierr.ErrUsage("name cannot be empty")
			}
			patch["name/full"] = newName
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		card, err := c.Contact(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if cmd.Flags().Changed("name") && card["name"] == nil {
			delete(patch, "name/full")
			patch["name"] = map[string]any{"@type": "Name", "full": newName}
		}
		// Merge into the maps so unrelated addresses, phone numbers and metadata survive.
		for flag, value := range map[string]string{"email": newEmail, "phone": newPhone} {
			if !cmd.Flags().Changed(flag) {
				continue
			}
			prop, key, field, typ := "emails", "email", "address", "EmailAddress"
			if flag == "phone" {
				prop, key, field, typ = "phones", "phone", "number", "Phone"
			} else if value != "" {
				addresses, parseErr := jmap.ParseAddresses(value)
				if parseErr != nil {
					return parseErr
				}
				if len(addresses) != 1 {
					return apierr.ErrUsage("--email must contain one address")
				}
				value = addresses[0].Email
			}
			values, _ := card[prop].(map[string]any)
			if values == nil {
				values = map[string]any{}
			}
			if value == "" {
				delete(values, key)
			} else {
				values[key] = map[string]any{"@type": typ, field: value}
			}
			patch[prop] = values
		}
		if len(patch) == 0 {
			return apierr.ErrUsage("provide --name, --email or --phone")
		}
		if err = c.Call(cmd.Context(), jmap.Contacts, "ContactCard/set", map[string]any{"update": map[string]any{args[0]: patch}}, nil); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Contact updated")
	}}
	update.Flags().StringVar(&newName, "name", "", "Full name")
	update.Flags().StringVar(&newEmail, "email", "", "CLI email field (empty removes it)")
	update.Flags().StringVar(&newPhone, "phone", "", "CLI phone field (empty removes it)")
	p.AddCommand(update)
	var yes bool
	del := &cobra.Command{Use: "delete ID", Short: "Permanently delete a contact", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return apierr.ErrUsage("contact deletion requires --yes")
		}
		c, err := a.api(cmd.Context())
		if err != nil {
			return err
		}
		if err = c.Call(cmd.Context(), jmap.Contacts, "ContactCard/set", map[string]any{"destroy": args}, nil); err != nil {
			return err
		}
		return a.mutation(cmd, map[string]string{"id": args[0]}, "Contact deleted")
	}}
	del.Flags().BoolVar(&yes, "yes", false, "Confirm deletion")
	p.AddCommand(del)
	return p
}
func contactLine(card jmap.ContactCard) string {
	name := ""
	if n, ok := card["name"].(map[string]any); ok {
		name, _ = n["full"].(string)
		if name == "" {
			if components, ok := n["components"].([]any); ok {
				for _, part := range components {
					if p, ok := part.(map[string]any); ok {
						if value, ok := p["value"].(string); ok {
							name += value + " "
						}
					}
				}
			}
		}
	}
	var emails []string
	if fields, ok := card["emails"].(map[string]any); ok {
		for _, field := range fields {
			if e, ok := field.(map[string]any); ok {
				if address, ok := e["address"].(string); ok {
					emails = append(emails, address)
				}
			}
		}
	}
	return fmt.Sprintf("%v\t%s\t%s", card["id"], name, strings.Join(emails, ", "))
}
