package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClientUpsertPluginRoute(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		return jsonResponse(http.StatusOK, `{"route_id":"plugin:alice:calendar:oauth","username":"alice","kind":"plugin","plugin_id":"calendar","hostname":"oauth.example.com","url":"https://oauth.example.com","local_port":19000,"enabled":true}`), nil
	})}}
	route, err := client.UpsertPluginRoute(t.Context(), "alice", "calendar", "oauth", 19000)
	if err != nil {
		t.Fatalf("UpsertPluginRoute: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/users/alice/routes" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	for _, want := range []string{`"plugin_id":"calendar"`, `"hostname_hint":"oauth"`, `"local_port":19000`} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("body %q missing %s", gotBody, want)
		}
	}
	if route.ID != "plugin:alice:calendar:oauth" || route.Hostname != "oauth.example.com" || route.LocalPort != 19000 {
		t.Fatalf("unexpected route: %+v", route)
	}
}

func TestClientDeletePluginRoute(t *testing.T) {
	var gotMethod, gotPath string
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		return jsonResponse(http.StatusOK, `{"ok":true}`), nil
	})}}
	if err := client.DeletePluginRoute(t.Context(), "alice", "plugin:alice:calendar:oauth"); err != nil {
		t.Fatalf("DeletePluginRoute: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/users/alice/routes/plugin:alice:calendar:oauth" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func TestClientPluginRouteError(t *testing.T) {
	client := Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusForbidden, `{"error":"forbidden"}`), nil
	})}}
	_, err := client.UpsertPluginRoute(t.Context(), "alice", "calendar", "oauth", 19000)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
