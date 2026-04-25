package network

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeDoer struct {
	methods []string
	t       *testing.T
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	f.t.Helper()
	if r.Header.Get("Authorization") != "Bearer token" {
		f.t.Fatalf("missing auth header: %q", r.Header.Get("Authorization"))
	}
	f.methods = append(f.methods, r.Method)
	var body any
	switch r.Method {
	case http.MethodGet:
		body = map[string]any{
			"success": true,
			"result":  []DNSRecord{{ID: "id1", Type: "CNAME", Name: "*.example.com", Content: "old"}},
		}
	case http.MethodPost, http.MethodPut:
		body = map[string]any{
			"success": true,
			"result":  DNSRecord{ID: "id2", Type: "CNAME", Name: "*.example.com", Content: "abc.cfargotunnel.com"},
		}
	default:
		f.t.Fatalf("unexpected method %s", r.Method)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}

func TestHTTPDNSClientListCreateUpdate(t *testing.T) {
	doer := &fakeDoer{t: t}
	client := NewHTTPDNSClient(doer)
	records, err := client.ListRecords(context.Background(), "zone", "token", "CNAME", "*.example.com")
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	if len(records) != 1 || records[0].ID != "id1" {
		t.Fatalf("unexpected records: %+v", records)
	}
	if _, err := client.CreateRecord(context.Background(), "zone", "token", DNSRecord{Type: "CNAME"}); err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}
	if _, err := client.UpdateRecord(context.Background(), "zone", "token", "id1", DNSRecord{Type: "CNAME"}); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if len(doer.methods) != 3 {
		t.Fatalf("expected three calls, got %v", doer.methods)
	}
}

func TestHTTPDNSClientReportsCloudflareErrors(t *testing.T) {
	client := NewHTTPDNSClient(doerFunc(func(*http.Request) (*http.Response, error) {
		data, err := json.Marshal(map[string]any{
			"success": false,
			"errors":  []map[string]string{{"message": "bad token"}},
		})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	}))
	if _, err := client.ListRecords(context.Background(), "zone", "token", "CNAME", "*.example.com"); err == nil {
		t.Fatal("expected Cloudflare API error")
	}
}

func TestHTTPDNSClientReportsHTTPError(t *testing.T) {
	client := NewHTTPDNSClient(doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}))
	if _, err := client.ListRecords(context.Background(), "zone", "token", "CNAME", "*.example.com"); err == nil {
		t.Fatal("expected HTTP error")
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}
