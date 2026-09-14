package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string

	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register sends the agent identity and capacity to dns-control.
func (c *Client) Register(
	ctx context.Context,
	agentID string,
	sniCapacity int,
	routeCapacity int,
) error {

	reqBody := RegisterRequest{
		AgentID: agentID,
	}

	reqBody.Capacity.SNI = sniCapacity
	reqBody.Capacity.Route = routeCapacity

	return c.post(
		ctx,
		"/api/v1/agent/register",
		reqBody,
		nil,
	)
}

// Poll requests new leases from dns-control.
func (c *Client) Poll(
	ctx context.Context,
	agentID string,
	max int,
) (PollResponse, error) {

	reqBody := PollRequest{
		AgentID: agentID,
		Max:     max,
	}

	var resp PollResponse

	if err := c.post(
		ctx,
		"/api/v1/agent/poll",
		reqBody,
		&resp,
	); err != nil {
		return PollResponse{}, err
	}

	return resp, nil
}

// Heartbeat renews active leases.
func (c *Client) Heartbeat(
	ctx context.Context,
	agentID string,
	leaseIDs []string,
) error {

	reqBody := HeartbeatRequest{
		AgentID:  agentID,
		LeaseIDs: leaseIDs,
	}

	return c.post(
		ctx,
		"/api/v1/agent/heartbeat",
		reqBody,
		nil,
	)
}

// Report tells dns-control whether a lease became active.
func (c *Client) Report(
	ctx context.Context,
	reqBody ReportRequest,
) error {

	return c.post(
		ctx,
		"/api/v1/agent/report",
		reqBody,
		nil,
	)
}

func (c *Client) post(
	ctx context.Context,
	path string,
	body any,
	result any,
) error {

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf(
			"marshal %s: %w",
			path,
			err,
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+path,
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf(
			"create request %s: %w",
			path,
			err,
		)
	}

	req.Header.Set(
		"Content-Type",
		"application/json",
	)

	c.authorize(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(
			"request %s: %w",
			path,
			err,
		)
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 ||
		resp.StatusCode >= 300 {

		return fmt.Errorf(
			"%s returned HTTP %d",
			path,
			resp.StatusCode,
		)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf(
				"decode %s response: %w",
				path,
				err,
			)
		}
	}

	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.token == "" {
		return
	}

	req.Header.Set(
		"Authorization",
		"Bearer "+c.token,
	)
}
