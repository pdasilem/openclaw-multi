package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/pdasilem/openclaw-multi/internal/state"
)

type Client struct {
	SocketPath string
	HTTP       *http.Client
}

func (c Client) EnableUserGateway(ctx context.Context, user state.User) (state.Route, error) {
	var resp RouteResponse
	if err := c.do(ctx, http.MethodPost, "/users/"+user.Username+"/gateway-route", RouteRequest{LocalPort: user.Port}, &resp); err != nil {
		return state.Route{}, err
	}
	return responseRoute(resp), nil
}

func (c Client) DisableUserRoutes(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodPost, "/users/"+username+"/disable", nil, nil)
}

func (c Client) DeleteUserRoutes(ctx context.Context, username string) error {
	return c.do(ctx, http.MethodDelete, "/users/"+username+"/gateway-route", nil, nil)
}

func (c Client) do(ctx context.Context, method, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		socket := c.SocketPath
		if socket == "" {
			socket = DefaultSocketPath
		}
		client = &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}}}
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 400 {
		var er ErrorResponse
		_ = json.NewDecoder(res.Body).Decode(&er)
		if er.Error == "" {
			er.Error = res.Status
		}
		return fmt.Errorf("overlay-api %s %s: %s", method, path, er.Error)
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func responseRoute(r RouteResponse) state.Route {
	kind := state.RouteKind(r.Kind)
	return state.Route{ID: r.RouteID, Username: r.Username, Kind: kind, PluginID: r.PluginID, LocalPort: r.LocalPort, Hostname: r.Hostname, Enabled: r.Enabled}
}
