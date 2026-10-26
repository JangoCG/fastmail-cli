package jmap

import "context"

type Changes struct {
	OldState       string   `json:"oldState"`
	NewState       string   `json:"newState"`
	HasMoreChanges bool     `json:"hasMoreChanges"`
	Created        []string `json:"created"`
	Updated        []string `json:"updated"`
	Destroyed      []string `json:"destroyed"`
}

func (c *Client) EmailState(ctx context.Context) (string, error) {
	var r GetResult[Email]
	err := c.Call(ctx, Mail, "Email/get", map[string]any{"ids": []string{}, "properties": []string{"id"}}, &r)
	return r.State, err
}
func (c *Client) Changes(ctx context.Context, state string) (Changes, error) {
	var r Changes
	err := c.Call(ctx, Mail, "Email/changes", map[string]any{"sinceState": state, "maxChanges": 500}, &r)
	return r, err
}
