package forgeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Team is the wire-level representation of /api/v2/teams/.
type Team struct {
	ID           int64  `json:"id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Organization int64  `json:"organization"`
}

func (c *Client) GetTeam(ctx context.Context, id int64) (*Team, error) {
	var t Team
	if err := c.do(ctx, "GET", "/api/v2/teams/"+strconv.FormatInt(id, 10)+"/", nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) FindTeamByName(ctx context.Context, name string) (*Team, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/teams/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Team name %q matched %d records", name, lr.Count)
	}
	var t Team
	if err := json.Unmarshal(lr.Results[0], &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) CreateTeam(ctx context.Context, t *Team) (*Team, error) {
	var out Team
	if err := c.do(ctx, "POST", "/api/v2/teams/", t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateTeam(ctx context.Context, id int64, t *Team) (*Team, error) {
	var out Team
	path := "/api/v2/teams/" + strconv.FormatInt(id, 10) + "/"
	if err := c.do(ctx, "PATCH", path, t, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteTeam(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/teams/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// ResolveUser resolves a Forge username to an ID. Returns -1 if not found.
func (c *Client) ResolveUser(ctx context.Context, username string) (int64, error) {
	q := url.Values{}
	q.Set("username", username)
	q.Set("page_size", "2")

	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/users/?"+q.Encode(), nil, &lr); err != nil {
		return -1, err
	}
	if lr.Count == 0 {
		return -1, nil
	}
	if lr.Count > 1 {
		return -1, fmt.Errorf("ambiguous username %q matched %d users", username, lr.Count)
	}
	var item struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(lr.Results[0], &item); err != nil {
		return -1, err
	}
	return item.ID, nil
}

// ListTeamUsers returns the user IDs currently on a team.
func (c *Client) ListTeamUsers(ctx context.Context, teamID int64) ([]int64, error) {
	path := fmt.Sprintf("/api/v2/teams/%d/users/?page_size=200", teamID)
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

// AssociateTeamUser POSTs {"id": userID} to /teams/{id}/users/.
func (c *Client) AssociateTeamUser(ctx context.Context, teamID, userID int64) error {
	path := fmt.Sprintf("/api/v2/teams/%d/users/", teamID)
	return c.do(ctx, "POST", path, map[string]int64{"id": userID}, nil)
}

// DisassociateTeamUser POSTs {"id": userID, "disassociate": true}.
func (c *Client) DisassociateTeamUser(ctx context.Context, teamID, userID int64) error {
	path := fmt.Sprintf("/api/v2/teams/%d/users/", teamID)
	return c.do(ctx, "POST", path, map[string]any{"id": userID, "disassociate": true}, nil)
}
