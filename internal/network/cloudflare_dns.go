package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DNSClient is the Cloudflare DNS API subset used by Phase 5.
type DNSClient interface {
	ListRecords(ctx context.Context, zoneID, token, recordType, name string) ([]DNSRecord, error)
	CreateRecord(ctx context.Context, zoneID, token string, record DNSRecord) (DNSRecord, error)
	UpdateRecord(ctx context.Context, zoneID, token, id string, record DNSRecord) (DNSRecord, error)
}

// Doer is implemented by *http.Client.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPDNSClient talks to the Cloudflare v4 DNS API.
type HTTPDNSClient struct {
	Client  Doer
	BaseURL string
}

// NewHTTPDNSClient returns a Cloudflare DNS client using client.
func NewHTTPDNSClient(client Doer) *HTTPDNSClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPDNSClient{Client: client, BaseURL: "https://api.cloudflare.com/client/v4"}
}

func (c *HTTPDNSClient) ListRecords(
	ctx context.Context,
	zoneID, token, recordType, name string,
) ([]DNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records?type=%s&name=%s", c.baseURL(), zoneID, recordType, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setCloudflareHeaders(req, token)
	var resp struct {
		Success bool        `json:"success"`
		Result  []DNSRecord `json:"result"`
		Errors  []cfError   `json:"errors"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("cloudflare list dns records: %s", formatCloudflareErrors(resp.Errors))
	}
	return resp.Result, nil
}

func (c *HTTPDNSClient) CreateRecord(
	ctx context.Context,
	zoneID, token string,
	record DNSRecord,
) (DNSRecord, error) {
	return c.writeRecord(ctx, http.MethodPost, fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL(), zoneID), token, record)
}

func (c *HTTPDNSClient) UpdateRecord(
	ctx context.Context,
	zoneID, token, id string,
	record DNSRecord,
) (DNSRecord, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL(), zoneID, id)
	return c.writeRecord(ctx, http.MethodPut, url, token, record)
}

func (c *HTTPDNSClient) writeRecord(
	ctx context.Context,
	method, url, token string,
	record DNSRecord,
) (DNSRecord, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return DNSRecord{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return DNSRecord{}, err
	}
	setCloudflareHeaders(req, token)
	var resp struct {
		Success bool      `json:"success"`
		Result  DNSRecord `json:"result"`
		Errors  []cfError `json:"errors"`
	}
	if err := c.do(req, &resp); err != nil {
		return DNSRecord{}, err
	}
	if !resp.Success {
		return DNSRecord{}, fmt.Errorf("cloudflare write dns record: %s", formatCloudflareErrors(resp.Errors))
	}
	return resp.Result, nil
}

func (c *HTTPDNSClient) do(req *http.Request, out any) error {
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare http %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode cloudflare response: %w", err)
	}
	return nil
}

func (c *HTTPDNSClient) baseURL() string {
	if c.BaseURL == "" {
		return "https://api.cloudflare.com/client/v4"
	}
	return c.BaseURL
}

type cfError struct {
	Message string `json:"message"`
}

func setCloudflareHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
}

func formatCloudflareErrors(errors []cfError) string {
	if len(errors) == 0 {
		return "unknown error"
	}
	return errors[0].Message
}
