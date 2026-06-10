package forailapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Schedule is the Forail wire-level Schedule. Forail schedules attach to
// any "unified job template" (JobTemplate, WorkflowJobTemplate,
// ProjectUpdate, InventoryUpdate, SystemJob). MVP supports JobTemplate.
//
// ExtraData is RawMessage because Forail accepts a YAML/JSON string on
// POST/PATCH but always returns a JSON object on GET. RawMessage lets
// us send/receive both.
type Schedule struct {
	ID                 int64           `json:"id,omitempty"`
	Name               string          `json:"name"`
	Description        string          `json:"description,omitempty"`
	RRule              string          `json:"rrule"`
	Enabled            bool            `json:"enabled"`
	ExtraData          json.RawMessage `json:"extra_data,omitempty"`
	UnifiedJobTemplate int64           `json:"unified_job_template"`
	NextRun            string          `json:"next_run,omitempty"`
}

func (c *Client) GetSchedule(ctx context.Context, id int64) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, "GET", "/api/v2/schedules/"+strconv.FormatInt(id, 10)+"/", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) FindScheduleByName(ctx context.Context, name string) (*Schedule, error) {
	q := url.Values{}
	q.Set("name", name)
	q.Set("page_size", "2")
	var lr listResult
	if err := c.do(ctx, "GET", "/api/v2/schedules/?"+q.Encode(), nil, &lr); err != nil {
		return nil, err
	}
	if lr.Count == 0 {
		return nil, nil
	}
	if lr.Count > 1 {
		return nil, fmt.Errorf("ambiguous Schedule name %q matched %d records", name, lr.Count)
	}
	var sch Schedule
	if err := json.Unmarshal(lr.Results[0], &sch); err != nil {
		return nil, err
	}
	return &sch, nil
}

// CreateSchedule attaches a schedule to a JobTemplate. Forail accepts
// POST to the per-JobTemplate sub-collection or the global one; the
// global endpoint requires unified_job_template to be set, which we do.
func (c *Client) CreateSchedule(ctx context.Context, sch *Schedule) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, "POST", "/api/v2/schedules/", sch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) UpdateSchedule(ctx context.Context, id int64, sch *Schedule) (*Schedule, error) {
	var out Schedule
	if err := c.do(ctx, "PATCH", "/api/v2/schedules/"+strconv.FormatInt(id, 10)+"/", sch, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteSchedule(ctx context.Context, id int64) error {
	err := c.do(ctx, "DELETE", "/api/v2/schedules/"+strconv.FormatInt(id, 10)+"/", nil, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}
