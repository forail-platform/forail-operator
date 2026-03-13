// Package forgeapi is a thin HTTP client over the Forge REST API.
//
// Auth: HTTP Basic (admin user) for now. A token-based path can be added
// later — Forge supports OAuth2 PATs via /api/v2/tokens/.
//
// Only the minimum surface needed by the operator is wrapped. Everything
// is JSON in / JSON out, no codegen.
package forgeapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Client struct {
	BaseURL string
	// Token is an OAuth2 Bearer (PAT) issued via `forge-manage
	// create_oauth2_token` or POST /api/v2/users/{id}/personal_tokens/.
	// Forge does not accept HTTP Basic on /api/v2/.
	Token string
	// HostHeader overrides the Host header on outbound requests. Needed
	// when reaching Forge through an Ingress that routes by hostname
	// (e.g. forge.local) but the BaseURL uses a node IP.
	HostHeader string
	HTTP       *http.Client
}

// New builds a client. Set insecureSkipVerify=true for self-signed TLS
// (test clusters); false in production with trusted CA.
func New(baseURL, token, hostHeader string, insecureSkipVerify bool) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		HostHeader: hostHeader,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
			},
		},
	}
}

// errAPI surfaces HTTP failures with the response body for diagnosis.
type errAPI struct {
	Status int
	URL    string
	Body   string
}

func (e *errAPI) Error() string {
	return fmt.Sprintf("forge api %s: %d: %s", e.URL, e.Status, e.Body)
}

// IsNotFound returns true if err is a 404 from the API.
func IsNotFound(err error) bool {
	if e, ok := err.(*errAPI); ok {
		return e.Status == http.StatusNotFound
	}
	return false
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if c.HostHeader != "" {
		req.Host = c.HostHeader
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return &errAPI{Status: resp.StatusCode, URL: req.URL.String(), Body: string(respBody)}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w (body=%s)", err, string(respBody))
		}
	}
	return nil
}

// listResult is the standard Forge list envelope.
type listResult struct {
	Count   int               `json:"count"`
	Results []json.RawMessage `json:"results"`
}

// resolveByName looks up a single entity ID by exact-name match.
// Returns -1 if not found.
func (c *Client) resolveByName(ctx context.Context, resource, name string) (int64, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", fmt.Sprintf("/api/v2/%s/?%s", resource, q.Encode()), nil, &lr); err != nil {
		return -1, err
	}
	if lr.Count == 0 {
		return -1, nil
	}
	if lr.Count > 1 {
		return -1, fmt.Errorf("ambiguous %s name %q matched %d records", resource, name, lr.Count)
	}
	var item struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(lr.Results[0], &item); err != nil {
		return -1, err
	}
	return item.ID, nil
}

func (c *Client) ResolveInventory(ctx context.Context, name string) (int64, error) {
	return c.resolveByName(ctx, "inventories", name)
}

func (c *Client) ResolveProject(ctx context.Context, name string) (int64, error) {
	return c.resolveByName(ctx, "projects", name)
}

func (c *Client) ResolveOrganization(ctx context.Context, name string) (int64, error) {
	return c.resolveByName(ctx, "organizations", name)
}

func (c *Client) ResolveCredential(ctx context.Context, name string) (int64, error) {
	return c.resolveByName(ctx, "credentials", name)
}

// JobTemplate is the wire-level representation. Fields the operator does
// not control are omitted to keep the diff space small.
type JobTemplate struct {
	ID                    int64  `json:"id,omitempty"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	JobType               string `json:"job_type,omitempty"`
	Inventory             int64  `json:"inventory"`
	Project               int64  `json:"project"`
	Playbook              string `json:"playbook"`
	Forks                 int32  `json:"forks,omitempty"`
	Verbosity             int32  `json:"verbosity,omitempty"`
	ExtraVars             string `json:"extra_vars,omitempty"`
	Limit                 string `json:"limit,omitempty"`
	AskInventoryOnLaunch  bool   `json:"ask_inventory_on_launch,omitempty"`
	AskCredentialOnLaunch bool   `json:"ask_credential_on_launch,omitempty"`
	AskVariablesOnLaunch  bool   `json:"ask_variables_on_launch,omitempty"`
	AskLimitOnLaunch      bool   `json:"ask_limit_on_launch,omitempty"`
}

func (c *Client) GetJobTemplate(ctx context.Context, id int64) (*JobTemplate, error) {
	var jt JobTemplate
	if err := c.do(ctx, "GET", "/api/v2/job_templates/"+strconv.FormatInt(id, 10)+"/", nil, &jt); err != nil {
		return nil, err
	}
	return &jt, nil
}

// FindJobTemplateByName returns nil if not found.
func (c *Client) FindJobTemplateByName(ctx context.Context, name string) (*JobTemplate, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/job_templates/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous JobTemplate name %q matched %d records", name, lr.Count)
	}
	var jt JobTemplate
	if err := json.Unmarshal(lr.Results[0], &jt); err != nil {
		return nil, err
	}
	return &jt, nil
}

func (c *Client) CreateJobTemplate(ctx context.Context, jt *JobTemplate) (*JobTemplate, error) {
	var out JobTemplate
	if err := c.do(ctx, "POST", "/api/v2/job_templates/", jt, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateJobTemplate(ctx context.Context, id int64, jt *JobTemplate) (*JobTemplate, error) {
	var out JobTemplate
	path := "/api/v2/job_templates/" + strconv.FormatInt(id, 10) + "/"
	if err := c.do(ctx, "PATCH", path, jt, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteJobTemplate(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/job_templates/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// AssociateCredential POSTs to /api/v2/job_templates/{id}/credentials/
// with {"id": credID} to attach a credential.
func (c *Client) AssociateCredential(ctx context.Context, jobTemplateID, credentialID int64) error {
	path := fmt.Sprintf("/api/v2/job_templates/%d/credentials/", jobTemplateID)
	return c.do(ctx, "POST", path, map[string]int64{"id": credentialID}, nil)
}

// DisassociateCredential POSTs with {"id": id, "disassociate": true}.
func (c *Client) DisassociateCredential(ctx context.Context, jobTemplateID, credentialID int64) error {
	path := fmt.Sprintf("/api/v2/job_templates/%d/credentials/", jobTemplateID)
	return c.do(ctx, "POST", path, map[string]any{"id": credentialID, "disassociate": true}, nil)
}

// ListJobTemplateCredentials returns the credentials currently attached.
func (c *Client) ListJobTemplateCredentials(ctx context.Context, jobTemplateID int64) ([]int64, error) {
	path := fmt.Sprintf("/api/v2/job_templates/%d/credentials/", jobTemplateID)
	var lr listResult
	if err := c.do(ctx, "GET", path, nil, &lr); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(lr.Results))
	for _, r := range lr.Results {
		var item struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(r, &item); err != nil {
			return nil, err
		}
		ids = append(ids, item.ID)
	}
	return ids, nil
}
