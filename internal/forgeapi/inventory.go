package forgeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Inventory is the Forge inventory wire-level type.
type Inventory struct {
	ID           int64  `json:"id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Organization int64  `json:"organization"`
	Variables    string `json:"variables,omitempty"`
	// HostsCount and GroupsCount come from the GET response (read-only).
	HostsCount  int32 `json:"total_hosts,omitempty"`
	GroupsCount int32 `json:"total_groups,omitempty"`
}

// Host is the Forge host wire-level type.
type Host struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Inventory   int64  `json:"inventory,omitempty"`
	Enabled     bool   `json:"enabled"`
	Variables   string `json:"variables,omitempty"`
}

// Group is the Forge group wire-level type.
type Group struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Inventory   int64  `json:"inventory,omitempty"`
	Variables   string `json:"variables,omitempty"`
}

// --- Inventory CRUD ---

func (c *Client) GetInventory(ctx context.Context, id int64) (*Inventory, error) {
	var inv Inventory
	if err := c.do(ctx, "GET", "/api/v2/inventories/"+strconv.FormatInt(id, 10)+"/", nil, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (c *Client) FindInventoryByName(ctx context.Context, name string) (*Inventory, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")
	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/inventories/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Inventory name %q matched %d records", name, lr.Count)
	}
	var inv Inventory
	if err := json.Unmarshal(lr.Results[0], &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (c *Client) CreateInventory(ctx context.Context, inv *Inventory) (*Inventory, error) {
	var out Inventory
	if err := c.do(ctx, "POST", "/api/v2/inventories/", inv, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateInventory(ctx context.Context, id int64, inv *Inventory) (*Inventory, error) {
	var out Inventory
	if err := c.do(ctx, "PATCH", "/api/v2/inventories/"+strconv.FormatInt(id, 10)+"/", inv, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteInventory(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/inventories/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// --- Hosts ---

// ListHosts returns all hosts in the given inventory.
func (c *Client) ListHosts(ctx context.Context, inventoryID int64) ([]Host, error) {
	out := []Host{}
	page := 1
	for {
		var lr listResult
		path := fmt.Sprintf("/api/v2/inventories/%d/hosts/?page=%d&page_size=200", inventoryID, page)
		if err := c.do(ctx, "GET", path, nil, &lr); err != nil {
			return nil, err
		}
		for _, raw := range lr.Results {
			var h Host
			if err := json.Unmarshal(raw, &h); err != nil {
				return nil, err
			}
			out = append(out, h)
		}
		if len(lr.Results) < 200 {
			break
		}
		page++
	}
	return out, nil
}

// CreateHost adds a host to the given inventory.
func (c *Client) CreateHost(ctx context.Context, inventoryID int64, h *Host) (*Host, error) {
	h.Inventory = inventoryID
	var out Host
	path := fmt.Sprintf("/api/v2/inventories/%d/hosts/", inventoryID)
	if err := c.do(ctx, "POST", path, h, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateHost(ctx context.Context, id int64, h *Host) (*Host, error) {
	var out Host
	if err := c.do(ctx, "PATCH", fmt.Sprintf("/api/v2/hosts/%d/", id), h, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteHost(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v2/hosts/%d/", id), nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// --- Groups ---

func (c *Client) ListGroups(ctx context.Context, inventoryID int64) ([]Group, error) {
	out := []Group{}
	page := 1
	for {
		var lr listResult
		path := fmt.Sprintf("/api/v2/inventories/%d/groups/?page=%d&page_size=200", inventoryID, page)
		if err := c.do(ctx, "GET", path, nil, &lr); err != nil {
			return nil, err
		}
		for _, raw := range lr.Results {
			var g Group
			if err := json.Unmarshal(raw, &g); err != nil {
				return nil, err
			}
			out = append(out, g)
		}
		if len(lr.Results) < 200 {
			break
		}
		page++
	}
	return out, nil
}

func (c *Client) CreateGroup(ctx context.Context, inventoryID int64, g *Group) (*Group, error) {
	g.Inventory = inventoryID
	var out Group
	path := fmt.Sprintf("/api/v2/inventories/%d/groups/", inventoryID)
	if err := c.do(ctx, "POST", path, g, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateGroup(ctx context.Context, id int64, g *Group) (*Group, error) {
	var out Group
	if err := c.do(ctx, "PATCH", fmt.Sprintf("/api/v2/groups/%d/", id), g, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteGroup(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", fmt.Sprintf("/api/v2/groups/%d/", id), nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// --- Group <-> Host membership ---

// AssociateHostWithGroup adds a host to a group.
func (c *Client) AssociateHostWithGroup(ctx context.Context, groupID, hostID int64) error {
	path := fmt.Sprintf("/api/v2/groups/%d/hosts/", groupID)
	return c.do(ctx, "POST", path, map[string]int64{"id": hostID}, nil)
}

func (c *Client) DisassociateHostFromGroup(ctx context.Context, groupID, hostID int64) error {
	path := fmt.Sprintf("/api/v2/groups/%d/hosts/", groupID)
	return c.do(ctx, "POST", path, map[string]any{"id": hostID, "disassociate": true}, nil)
}

func (c *Client) ListGroupHosts(ctx context.Context, groupID int64) ([]int64, error) {
	out := []int64{}
	page := 1
	for {
		var lr listResult
		path := fmt.Sprintf("/api/v2/groups/%d/hosts/?page=%d&page_size=200", groupID, page)
		if err := c.do(ctx, "GET", path, nil, &lr); err != nil {
			return nil, err
		}
		for _, raw := range lr.Results {
			var item struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			out = append(out, item.ID)
		}
		if len(lr.Results) < 200 {
			break
		}
		page++
	}
	return out, nil
}

// --- Group <-> Group (children) ---

func (c *Client) AssociateChildGroup(ctx context.Context, parentID, childID int64) error {
	path := fmt.Sprintf("/api/v2/groups/%d/children/", parentID)
	return c.do(ctx, "POST", path, map[string]int64{"id": childID}, nil)
}

func (c *Client) DisassociateChildGroup(ctx context.Context, parentID, childID int64) error {
	path := fmt.Sprintf("/api/v2/groups/%d/children/", parentID)
	return c.do(ctx, "POST", path, map[string]any{"id": childID, "disassociate": true}, nil)
}

func (c *Client) ListGroupChildren(ctx context.Context, groupID int64) ([]int64, error) {
	out := []int64{}
	page := 1
	for {
		var lr listResult
		path := fmt.Sprintf("/api/v2/groups/%d/children/?page=%d&page_size=200", groupID, page)
		if err := c.do(ctx, "GET", path, nil, &lr); err != nil {
			return nil, err
		}
		for _, raw := range lr.Results {
			var item struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			out = append(out, item.ID)
		}
		if len(lr.Results) < 200 {
			break
		}
		page++
	}
	return out, nil
}
