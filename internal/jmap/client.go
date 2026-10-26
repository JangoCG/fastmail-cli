// Package jmap implements the Fastmail JMAP transport (RFC 8620).
package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JangoCG/fastmail-cli/internal/apierr"
	"github.com/JangoCG/fastmail-cli/internal/version"
)

const (
	Core              = "urn:ietf:params:jmap:core"
	Mail              = "urn:ietf:params:jmap:mail"
	Submission        = "urn:ietf:params:jmap:submission"
	Contacts          = "urn:ietf:params:jmap:contacts"
	Vacation          = "urn:ietf:params:jmap:vacationresponse"
	Masked            = "https://www.fastmail.com/dev/maskedemail"
	DefaultSessionURL = "https://api.fastmail.com/jmap/session"
	maxResponse       = 32 << 20
)

type Account struct {
	Name                string                     `json:"name"`
	IsPersonal          bool                       `json:"isPersonal"`
	IsReadOnly          bool                       `json:"isReadOnly"`
	AccountCapabilities map[string]json.RawMessage `json:"accountCapabilities"`
}
type Session struct {
	Capabilities    map[string]json.RawMessage `json:"capabilities"`
	Accounts        map[string]Account         `json:"accounts"`
	PrimaryAccounts map[string]string          `json:"primaryAccounts"`
	Username        string                     `json:"username"`
	APIURL          string                     `json:"apiUrl"`
	DownloadURL     string                     `json:"downloadUrl"`
	UploadURL       string                     `json:"uploadUrl"`
	State           string                     `json:"state"`
}
type Client struct {
	HTTP       *http.Client
	SessionURL string
	Token      string
	AccountID  string
	session    *Session
}

func New(token, sessionURL, accountID string) *Client {
	if sessionURL == "" {
		sessionURL = DefaultSessionURL
	}
	return &Client{Token: token, SessionURL: sessionURL, AccountID: accountID, HTTP: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

// ValidateURL allows TLS and explicit loopback HTTP for isolated integration tests.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return apierr.ErrUsage("invalid server URL")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
		return nil
	}
	return apierr.ErrUsage("server URL must use HTTPS (HTTP is allowed only on loopback)")
}

func (c *Client) Discover(ctx context.Context) (*Session, error) {
	if c.session != nil {
		return c.session, nil
	}
	if strings.TrimSpace(c.Token) == "" {
		return nil, apierr.ErrAuth("Fastmail API token is missing")
	}
	data, err := c.request(ctx, http.MethodGet, c.SessionURL, "", nil, maxResponse)
	if err != nil {
		return nil, err
	}
	var s Session
	if err = json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode JMAP session: %w", err)
	}
	if err = ValidateURL(s.APIURL); err != nil {
		return nil, fmt.Errorf("invalid JMAP apiUrl: %w", err)
	}
	if _, ok := s.Capabilities[Core]; !ok {
		return nil, apierr.ErrAPI(0, "server does not advertise JMAP core")
	}
	c.session = &s
	return &s, nil
}

func (c *Client) Account(ctx context.Context, capability string) (string, error) {
	s, err := c.Discover(ctx)
	if err != nil {
		return "", err
	}
	if _, ok := s.Capabilities[capability]; !ok {
		return "", apierr.ErrForbidden("API token does not grant capability " + capability)
	}
	id := c.AccountID
	if id == "" {
		id = s.PrimaryAccounts[capability]
	}
	if id == "" {
		for candidate, account := range s.Accounts {
			if _, ok := account.AccountCapabilities[capability]; ok {
				if id != "" {
					return "", apierr.ErrUsage("multiple accounts support this capability; choose --account")
				}
				id = candidate
			}
		}
	}
	a, ok := s.Accounts[id]
	if !ok {
		return "", apierr.ErrUsage("no account for capability " + capability + "; choose --account from 'fastmail accounts'")
	}
	if _, ok = a.AccountCapabilities[capability]; !ok {
		return "", apierr.ErrForbidden("selected account does not support " + capability)
	}
	return id, nil
}

// Call validates the method response as well as per-object failures in /set.
// Mutations are never retried: a lost HTTP response does not prove a send failed.
func (c *Client) Call(ctx context.Context, capability, method string, args map[string]any, out any) error {
	id, err := c.Account(ctx, capability)
	if err != nil {
		return err
	}
	if args == nil {
		args = map[string]any{}
	}
	args["accountId"] = id
	using := []string{Core, capability}
	if capability == Submission {
		using = append(using, Mail)
	}
	req := map[string]any{"using": using, "methodCalls": []any{[]any{method, args, "0"}}}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data, err := c.request(ctx, http.MethodPost, c.session.APIURL, "application/json", bytes.NewReader(payload), maxResponse)
	if err != nil {
		return err
	}
	var resp struct {
		MethodResponses [][]json.RawMessage `json:"methodResponses"`
	}
	if err = json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("decode JMAP response: %w", err)
	}
	var result json.RawMessage
	for _, r := range resp.MethodResponses {
		if len(r) != 3 {
			return apierr.ErrAPI(0, "malformed JMAP method response")
		}
		var name, tag string
		if json.Unmarshal(r[0], &name) != nil || json.Unmarshal(r[2], &tag) != nil {
			return apierr.ErrAPI(0, "invalid JMAP response name or id")
		}
		if tag != "0" {
			continue
		}
		if name == "error" {
			var e MethodError
			if err = json.Unmarshal(r[1], &e); err != nil {
				return err
			}
			return e.Err(method)
		}
		if strings.HasSuffix(name, "/set") {
			if err = checkSet(name, r[1]); err != nil {
				return err
			}
		}
		if name == method {
			result = r[1]
		}
	}
	if result == nil {
		return apierr.ErrAPI(0, "missing JMAP response for "+method)
	}
	if out != nil {
		return json.Unmarshal(result, out)
	}
	return nil
}

type MethodError struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Properties  []string `json:"properties,omitempty"`
}

func (e MethodError) Err(method string) error {
	code := apierr.CodeAPI
	switch e.Type {
	case "notFound":
		code = apierr.CodeNotFound
	case "forbidden", "forbiddenToSend", "accountReadOnly":
		code = apierr.CodeForbidden
	case "stateMismatch", "alreadyExists":
		code = apierr.CodeConflict
	case "rateLimit", "rateLimitReached":
		code = apierr.CodeRateLimit
	case "invalidArguments", "invalidProperties":
		code = apierr.CodeValidation
	}
	return &apierr.Error{Code: code, Message: strings.TrimSpace(method + ": " + e.Type + " " + e.Description), Meta: map[string]any{"type": e.Type, "properties": e.Properties}}
}
func checkSet(method string, data json.RawMessage) error {
	var r struct {
		NotCreated   map[string]MethodError `json:"notCreated"`
		NotUpdated   map[string]MethodError `json:"notUpdated"`
		NotDestroyed map[string]MethodError `json:"notDestroyed"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	for _, failures := range []map[string]MethodError{r.NotCreated, r.NotUpdated, r.NotDestroyed} {
		for id, e := range failures {
			return &apierr.Error{Code: apierr.AsError(e.Err(method)).Code, Message: e.Err(method).Error(), Meta: map[string]any{"id": id, "type": e.Type, "result": data}}
		}
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, rawURL, contentType string, body io.Reader, limit int64) ([]byte, error) {
	if err := ValidateURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, apierr.ErrNetwork(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 401:
			return nil, apierr.ErrAuth("Fastmail rejected the API token")
		case 403:
			return nil, apierr.ErrForbidden("Fastmail denied access; check token permissions")
		case 404:
			return nil, apierr.ErrNotFound("resource", rawURL)
		case 429:
			return nil, &apierr.Error{Code: apierr.CodeRateLimit, Message: "Fastmail rate limit reached", Meta: map[string]any{"retryAfter": resp.Header.Get("Retry-After")}}
		}
		return nil, apierr.ErrAPI(resp.StatusCode, fmt.Sprintf("Fastmail HTTP %d (request was not retried)", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, apierr.ErrAPI(0, "Fastmail response exceeds size limit")
	}
	return data, nil
}

func (c *Client) Upload(ctx context.Context, name, contentType string, body io.Reader) (BodyPart, error) {
	id, err := c.Account(ctx, Mail)
	if err != nil {
		return BodyPart{}, err
	}
	u := strings.ReplaceAll(c.session.UploadURL, "{accountId}", url.PathEscape(id))
	if u == "" {
		return BodyPart{}, apierr.ErrAPI(0, "session has no uploadUrl")
	}
	data, err := c.request(ctx, http.MethodPost, u, contentType, body, maxResponse)
	if err != nil {
		return BodyPart{}, err
	}
	var upload struct {
		BlobID string `json:"blobId"`
		Type   string `json:"type"`
		Size   int64  `json:"size"`
	}
	if err = json.Unmarshal(data, &upload); err != nil {
		return BodyPart{}, err
	}
	if upload.BlobID == "" {
		return BodyPart{}, apierr.ErrAPI(0, "upload response has no blobId")
	}
	return BodyPart{BlobID: upload.BlobID, Type: upload.Type, Size: upload.Size, Name: name, Disposition: "attachment"}, nil
}
func (c *Client) Download(ctx context.Context, blobID, name, contentType string) ([]byte, error) {
	id, err := c.Account(ctx, Mail)
	if err != nil {
		return nil, err
	}
	u := strings.NewReplacer("{accountId}", url.PathEscape(id), "{blobId}", url.PathEscape(blobID), "{name}", url.PathEscape(name), "{type}", url.QueryEscape(contentType)).Replace(c.session.DownloadURL)
	if u == "" {
		return nil, apierr.ErrAPI(0, "session has no downloadUrl")
	}
	return c.request(ctx, http.MethodGet, u, "", nil, 128<<20)
}
