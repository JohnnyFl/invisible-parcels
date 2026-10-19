// Package emuclient talks to the NP emulator: NP v2.0 tracking and the demo admin API.
package emuclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"novapost/contract"
)

type Client struct {
	URL  string
	HTTP *http.Client
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("emulator %s %s: %s", method, path, resp.Status)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Track calls TrackingDocument.getStatusDocuments in batches of 100.
func (c *Client) Track(ctx context.Context, ttns []string) ([]contract.TrackingStatus, error) {
	var out []contract.TrackingStatus
	for start := 0; start < len(ttns); start += 100 {
		batch := ttns[start:min(start+100, len(ttns))]
		docs := make([]contract.Document, len(batch))
		for i, t := range batch {
			docs[i] = contract.Document{DocumentNumber: t}
		}
		var resp contract.Response[contract.TrackingStatus]
		err := c.do(ctx, http.MethodPost, "/v2.0/json/", contract.Envelope[contract.TrackingRequest]{
			APIKey: "demo", ModelName: "TrackingDocument", CalledMethod: "getStatusDocuments",
			MethodProperties: contract.TrackingRequest{Documents: docs},
		}, &resp)
		if err != nil {
			return nil, err
		}
		if !resp.Success {
			return nil, fmt.Errorf("getStatusDocuments: %v", resp.Errors)
		}
		out = append(out, resp.Data...)
	}
	return out, nil
}

func (c *Client) SetClock(ctx context.Context, virtualNow time.Time, speed float64) error {
	return c.do(ctx, http.MethodPost, "/admin/clock", map[string]any{"virtualNow": virtualNow, "speed": speed}, nil)
}

func (c *Client) Reset(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/admin/reset", nil, nil)
}

func (c *Client) LoadScenario(ctx context.Context, id int) error {
	return c.do(ctx, http.MethodPost, "/admin/scenarios/"+strconv.Itoa(id), nil, nil)
}

func (c *Client) Physical(ctx context.Context, location string) ([]string, error) {
	var out struct {
		TTNs []string `json:"ttns"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/physical?location="+url.QueryEscape(location), nil, &out)
	return out.TTNs, err
}

func (c *Client) OperatorEvent(ctx context.Context, e contract.Event) error {
	return c.do(ctx, http.MethodPost, "/admin/operator-event", e, nil)
}

// State returns the scenario the emulator has loaded (0 = none, e.g. after a restart).
func (c *Client) State(ctx context.Context) (int, error) {
	var out struct {
		Scenario int `json:"scenario"`
	}
	err := c.do(ctx, http.MethodGet, "/admin/state", nil, &out)
	return out.Scenario, err
}

// Added is one random parcel the emulator created and whose operator screen shows it.
type Added struct {
	TTN    string `json:"ttn"`
	Kind   string `json:"kind"`
	LookAt string `json:"lookAt"`
}

// AddRandom adds count random parcels to the running world.
func (c *Client) AddRandom(ctx context.Context, count int) ([]Added, error) {
	var out []Added
	err := c.do(ctx, http.MethodPost, "/admin/parcels/random", map[string]int{"count": count}, &out)
	return out, err
}
