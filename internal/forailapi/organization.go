package forailapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Organization is the wire-level representation of /api/v2/organizations/.
type Organization struct {
	ID                 int64  `json:"id,omitempty"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	MaxHosts           int32  `json:"max_hosts,omitempty"`
	DefaultEnvironment *int64 `json:"default_environment,omitempty"`
}

func (c *Client) GetOrganization(ctx context.Context, id int64) (*Organization, error) {
	var o Organization
	if err := c.do(ctx, "GET", "/api/v2/organizations/"+strconv.FormatInt(id, 10)+"/", nil, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// FindOrganizationByName returns nil if not found.
func (c *Client) FindOrganizationByName(ctx context.Context, name string) (*Organization, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/organizations/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Organization name %q matched %d records", name, lr.Count)
	}
	var o Organization
	if err := json.Unmarshal(lr.Results[0], &o); err != nil {
		return nil, err
	}
	return &o, nil
}

func (c *Client) CreateOrganization(ctx context.Context, o *Organization) (*Organization, error) {
	var out Organization
	if err := c.do(ctx, "POST", "/api/v2/organizations/", o, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateOrganization(ctx context.Context, id int64, o *Organization) (*Organization, error) {
	var out Organization
	path := "/api/v2/organizations/" + strconv.FormatInt(id, 10) + "/"
	if err := c.do(ctx, "PATCH", path, o, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteOrganization(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/organizations/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}
