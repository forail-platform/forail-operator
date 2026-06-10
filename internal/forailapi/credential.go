package forailapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// CredentialType is the Forail wire-level CredentialType (read-only from
// the operator's perspective — built-ins are populated by Forail).
type CredentialType struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
}

// ResolveCredentialType returns -1 if not found.
func (c *Client) ResolveCredentialType(ctx context.Context, name string) (int64, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")
	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/credential_types/?"+q.Encode(), nil, &lr); err != nil {
		return -1, err
	}
	if lr.Count == 0 {
		return -1, nil
	}
	if lr.Count > 1 {
		return -1, fmt.Errorf("ambiguous credential_type name %q matched %d records", name, lr.Count)
	}
	var ct CredentialType
	if err := json.Unmarshal(lr.Results[0], &ct); err != nil {
		return -1, err
	}
	return ct.ID, nil
}

// Credential is the Forail wire-level Credential.
type Credential struct {
	ID             int64             `json:"id,omitempty"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Organization   int64             `json:"organization"`
	CredentialType int64             `json:"credential_type"`
	Inputs         map[string]string `json:"inputs"`
}

func (c *Client) GetCredential(ctx context.Context, id int64) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "GET", "/api/v2/credentials/"+strconv.FormatInt(id, 10)+"/", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) FindCredentialByName(ctx context.Context, name string) (*Credential, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")
	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/credentials/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Credential name %q matched %d records", name, lr.Count)
	}
	var cred Credential
	if err := json.Unmarshal(lr.Results[0], &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *Client) CreateCredential(ctx context.Context, cred *Credential) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "POST", "/api/v2/credentials/", cred, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateCredential(ctx context.Context, id int64, cred *Credential) (*Credential, error) {
	var out Credential
	if err := c.do(ctx, "PATCH", "/api/v2/credentials/"+strconv.FormatInt(id, 10)+"/", cred, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteCredential(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/credentials/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}
