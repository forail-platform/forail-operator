package forailapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Project is the wire-level representation of /api/v2/projects/.
type Project struct {
	ID                    int64  `json:"id,omitempty"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	Organization          int64  `json:"organization"`
	ScmType               string `json:"scm_type"`
	ScmURL                string `json:"scm_url,omitempty"`
	ScmBranch             string `json:"scm_branch,omitempty"`
	ScmRefspec            string `json:"scm_refspec,omitempty"`
	Credential            *int64 `json:"credential,omitempty"`
	ScmClean              bool   `json:"scm_clean,omitempty"`
	ScmDeleteOnUpdate     bool   `json:"scm_delete_on_update,omitempty"`
	ScmUpdateOnLaunch     bool   `json:"scm_update_on_launch,omitempty"`
	ScmUpdateCacheTimeout int32  `json:"scm_update_cache_timeout,omitempty"`
	AllowOverride         bool   `json:"allow_override,omitempty"`
	Timeout               int32  `json:"timeout,omitempty"`
	DefaultEnvironment    *int64 `json:"default_environment,omitempty"`
	ScmRevision           string `json:"scm_revision,omitempty"`
}

func (c *Client) GetProject(ctx context.Context, id int64) (*Project, error) {
	var p Project
	if err := c.do(ctx, "GET", "/api/v2/projects/"+strconv.FormatInt(id, 10)+"/", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// FindProjectByName returns nil if not found.
func (c *Client) FindProjectByName(ctx context.Context, name string) (*Project, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/projects/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Project name %q matched %d records", name, lr.Count)
	}
	var p Project
	if err := json.Unmarshal(lr.Results[0], &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) CreateProject(ctx context.Context, p *Project) (*Project, error) {
	var out Project
	if err := c.do(ctx, "POST", "/api/v2/projects/", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateProject(ctx context.Context, id int64, p *Project) (*Project, error) {
	var out Project
	path := "/api/v2/projects/" + strconv.FormatInt(id, 10) + "/"
	if err := c.do(ctx, "PATCH", path, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProject(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/projects/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// ResolveExecutionEnvironment looks up an EE by name. Returns -1 if not found.
func (c *Client) ResolveExecutionEnvironment(ctx context.Context, name string) (int64, error) {
	return c.resolveByName(ctx, "execution_environments", name)
}
