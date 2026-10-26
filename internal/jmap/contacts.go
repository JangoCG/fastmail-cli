package jmap

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
)

type AddressBook struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"isDefault"`
	MyRights  map[string]bool `json:"myRights"`
}
type ContactCard map[string]any

func (c *Client) AddressBooks(ctx context.Context) ([]AddressBook, error) {
	var r GetResult[AddressBook]
	err := c.Call(ctx, Contacts, "AddressBook/get", nil, &r)
	return r.List, err
}
func (c *Client) ListContacts(ctx context.Context, text string, position, limit int) ([]ContactCard, QueryResult, error) {
	filter := map[string]any{}
	if text != "" {
		filter["text"] = text
	}
	var q QueryResult
	if err := c.Call(ctx, Contacts, "ContactCard/query", map[string]any{"filter": filter, "position": position, "limit": limit, "calculateTotal": true}, &q); err != nil {
		return nil, q, err
	}
	if len(q.IDs) == 0 {
		return []ContactCard{}, q, nil
	}
	var r GetResult[ContactCard]
	err := c.Call(ctx, Contacts, "ContactCard/get", map[string]any{"ids": q.IDs}, &r)
	return r.List, q, err
}
func (c *Client) Contact(ctx context.Context, id string) (ContactCard, error) {
	var r GetResult[ContactCard]
	if err := c.Call(ctx, Contacts, "ContactCard/get", map[string]any{"ids": []string{id}}, &r); err != nil {
		return nil, err
	}
	if len(r.List) != 1 {
		return nil, apierr.ErrNotFound("contact", id)
	}
	return r.List[0], nil
}
func UUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (c *Client) CreateContact(ctx context.Context, name, email, phone, book string) (string, error) {
	books, err := c.AddressBooks(ctx)
	if err != nil {
		return "", err
	}
	if book == "" {
		for _, b := range books {
			if b.IsDefault && b.MyRights["mayWrite"] {
				book = b.ID
				break
			}
		}
	}
	found := false
	for _, b := range books {
		if b.ID == book && b.MyRights["mayWrite"] {
			found = true
		}
	}
	if !found {
		return "", apierr.ErrUsage("choose a writable --addressbook ID from 'fastmail addressbook list'")
	}
	obj := ContactCard{"@type": "Card", "version": "1.0", "uid": UUID(), "name": map[string]any{"@type": "Name", "full": name}, "addressBookIds": map[string]bool{book: true}}
	if email != "" {
		obj["emails"] = map[string]any{"email": map[string]any{"@type": "EmailAddress", "address": email}}
	}
	if phone != "" {
		obj["phones"] = map[string]any{"phone": map[string]any{"@type": "Phone", "number": phone}}
	}
	var r SetResult
	if err = c.Call(ctx, Contacts, "ContactCard/set", map[string]any{"create": map[string]any{"contact": obj}}, &r); err != nil {
		return "", err
	}
	id := r.Created["contact"].ID
	if id == "" {
		return "", apierr.ErrAPI(0, "contact response has no ID")
	}
	return id, nil
}
