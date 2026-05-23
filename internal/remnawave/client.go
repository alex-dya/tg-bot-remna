package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	squadUUID  string
	httpClient *http.Client
}

func New(baseURL, token, squadUUID string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		squadUUID:  squadUUID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// envelope нужна, потому что Remnawave оборачивает payload в {"response": {...}}.
type envelope[T any] struct {
	Response T `json:"response"`
}

type CreateUserRequest struct {
	Username             string   `json:"username"`
	TrafficLimitBytes    int64    `json:"trafficLimitBytes"`
	TrafficLimitStrategy string   `json:"trafficLimitStrategy"`
	ExpireAt             string   `json:"expireAt"`
	HwidDeviceLimit      int      `json:"hwidDeviceLimit"`
	ActiveInternalSquads []string `json:"activeInternalSquads"`
	Description          string   `json:"description,omitempty"`
}

type User struct {
	UUID            string `json:"uuid"`
	Username        string `json:"username"`
	SubscriptionURL string `json:"subscriptionUrl"`
	Status          string `json:"status"`
	ExpireAt        string `json:"expireAt"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("remnawave %s %s: %d %s",
			method, path, resp.StatusCode, string(rawBody))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rawBody, out)
}

func (c *Client) CreateUser(ctx context.Context, username string, days, devices int, trafficGB int64) (*User, error) {
	var traffic int64
	if trafficGB > 0 {
		traffic = trafficGB * 1024 * 1024 * 1024
	}
	expire := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)

	req := CreateUserRequest{
		Username:             username,
		TrafficLimitBytes:    traffic,
		TrafficLimitStrategy: "NO_RESET",
		ExpireAt:             expire.Format(time.RFC3339),
		HwidDeviceLimit:      devices,
		ActiveInternalSquads: []string{c.squadUUID},
		Description:          "Corp employee, created by bot",
	}

	var env envelope[User]
	if err := c.do(ctx, http.MethodPost, "/api/users", req, &env); err != nil {
		return nil, err
	}
	return &env.Response, nil
}

func (c *Client) GetUser(ctx context.Context, uuid string) (*User, error) {
	var env envelope[User]
	if err := c.do(ctx, http.MethodGet, "/api/users/"+uuid, nil, &env); err != nil {
		return nil, err
	}
	return &env.Response, nil
}

func (c *Client) DeleteUser(ctx context.Context, uuid string) error {
	return c.do(ctx, http.MethodDelete, "/api/users/"+uuid, nil, nil)
}
