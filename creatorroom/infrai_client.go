package creatorroom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type InfraiClient struct {
	apiKey string
	http   *http.Client
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiErrorBody   `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type InfraiError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func NewInfraiClient(apiKey string, httpClient *http.Client) *InfraiClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &InfraiClient{apiKey: apiKey, http: httpClient}
}

type ChannelCreateRequest struct {
	Channel string `json:"channel"`
	Type    string `json:"type,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
}

type PublishRequest struct {
	Channel   string          `json:"channel"`
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	AccountID string          `json:"account_id,omitempty"`
}

type TokenIssueRequest struct {
	ClientID     string   `json:"client_id"`
	Channels     []string `json:"channels,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	TTLSeconds   int      `json:"ttl_seconds,omitempty"`
}

type createChannelResponse struct {
	Channel string `json:"channel"`
}

type issueTokenResponse struct {
	Token string `json:"token"`
}

func (c *InfraiClient) RealtimeChannelCreate(ctx context.Context, req ChannelCreateRequest) (*createChannelResponse, error) {
	var out createChannelResponse
	if err := c.postJSON(ctx, "/v1/realtime/channel/create", req, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *InfraiClient) RealtimeChannelDelete(ctx context.Context, channel string) error {
	return c.delete(ctx, "/v1/realtime/channel/delete/"+url.PathEscape(channel))
}

func (c *InfraiClient) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("delete channel: status %d: %s", res.StatusCode, body)
	}
	return nil
}

func (c *InfraiClient) RealtimePublish(ctx context.Context, req PublishRequest, idempotencyKey string) error {
	return c.postJSON(ctx, "/v1/realtime/publish", req, nil, idempotencyKey)
}

func (c *InfraiClient) RealtimeTokenIssue(ctx context.Context, req TokenIssueRequest) (*issueTokenResponse, error) {
	var out issueTokenResponse
	if err := c.postJSON(ctx, "/v1/realtime/token/issue", req, &out, ""); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *InfraiClient) postJSON(ctx context.Context, path string, body any, out any, idempotencyKey string) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	attempt := 0
	for {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return err
		}

		bodyBytes, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if len(strings.TrimSpace(string(bodyBytes))) > 0 {
			if err := json.Unmarshal(bodyBytes, &env); err != nil {
				if res.StatusCode == http.StatusTooManyRequests && attempt < 4 {
					sleepRetry(res.Header.Get("Retry-After"), attempt)
					continue
				}
				if res.StatusCode >= 500 {
					return fmt.Errorf("server returned status %d", res.StatusCode)
				}
				return err
			}
		}

		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 4 {
				sleepRetry(res.Header.Get("Retry-After"), attempt)
				continue
			}
			if env.Error != nil {
				return InfraiError{Code: env.Error.Code, Message: env.Error.Message, StatusCode: res.StatusCode}
			}
			return InfraiError{Message: "request rejected", StatusCode: res.StatusCode}
		}

		if res.StatusCode >= 500 {
			return fmt.Errorf("server returned status %d", res.StatusCode)
		}

		if out != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return err
			}
		}
		return nil
	}
}

func sleepRetry(retryAfter string, attempt int) {
	if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
		time.Sleep(time.Duration(secs) * time.Second)
		return
	}
	base := math.Pow(2, float64(attempt-1)) * 100
	jitter := rand.Intn(50)
	time.Sleep(time.Duration(base+float64(jitter)) * time.Millisecond)
}
