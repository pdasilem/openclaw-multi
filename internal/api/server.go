package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pdasilem/openclaw-multi/internal/audit"
	"github.com/pdasilem/openclaw-multi/internal/state"
)

var (
	errBadRequest = errors.New("bad request")
	errForbidden  = errors.New("forbidden")
)

type Auditor interface {
	Emit(audit.Event) error
}

type Server struct {
	Store   *state.Store
	Routes  RouteService
	Peers   PeerCredentialsProvider
	Auditor Auditor
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/users", s.users)
	mux.HandleFunc("/users/", s.userRoute)
	mux.HandleFunc("/cloudflared/reload", s.reload)
	return s.auditRequests(mux)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

func (s Server) auditRequests(next http.Handler) http.Handler {
	if s.Auditor == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		result := audit.ResultOk
		if rec.status >= http.StatusBadRequest {
			result = audit.ResultError
		}
		if err := s.Auditor.Emit(audit.Event{
			Actor:      peerActor(r),
			Action:     audit.ActionAPIRequest,
			Target:     r.Method + " " + r.URL.Path,
			Result:     result,
			DurationMs: time.Since(start).Milliseconds(),
			Details: map[string]any{
				"status": rec.status,
			},
		}); err != nil {
			return
		}
	})
}

func peerActor(r *http.Request) string {
	cred, ok := r.Context().Value(peerKey{}).(PeerCred)
	if !ok {
		return "unknown"
	}
	return "uid:" + strconv.Itoa(cred.UID)
}

func (s Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{OK: true})
}

func (s Server) users(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	users, err := s.Store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s Server) reload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.requireRoot(r); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	if err := s.Routes.Publish(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s Server) userRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/users/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	username := parts[0]
	if err := s.authorize(r, username); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "gateway-route":
		s.gateway(w, r, username)
	case len(parts) == 2 && parts[1] == "routes":
		s.routes(w, r, username)
	case len(parts) == 3 && parts[1] == "routes":
		s.routeByID(w, r, username, parts[2])
	case len(parts) == 4 && parts[1] == "routes" && parts[3] == "last-seen":
		s.lastSeen(w, r, username, parts[2])
	case len(parts) == 2 && parts[1] == "disable":
		s.enableDisable(w, r, username, false)
	case len(parts) == 2 && parts[1] == "enable":
		s.enableDisable(w, r, username, true)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s Server) gateway(w http.ResponseWriter, r *http.Request, username string) {
	switch r.Method {
	case http.MethodPost:
		var req RouteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		route, err := s.Routes.UpsertGateway(r.Context(), username, req.LocalPort)
		if err != nil {
			writeError(w, statusFor(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, routeResponse(route))
	case http.MethodDelete:
		if err := s.Routes.DeleteGateway(r.Context(), username); err != nil {
			writeError(w, statusFor(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s Server) routes(w http.ResponseWriter, r *http.Request, username string) {
	switch r.Method {
	case http.MethodGet:
		routes, err := s.Routes.Routes(r.Context(), username)
		if err != nil {
			writeError(w, statusFor(err), err.Error())
			return
		}
		out := make([]RouteResponse, 0, len(routes))
		for _, route := range routes {
			out = append(out, routeResponse(route))
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var req RouteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		route, err := s.Routes.UpsertPlugin(r.Context(), username, req.PluginID, req.HostnameHint, req.LocalPort)
		if err != nil {
			writeError(w, statusFor(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, routeResponse(route))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s Server) routeByID(w http.ResponseWriter, r *http.Request, username, id string) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.Routes.DeletePlugin(r.Context(), username, id); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s Server) lastSeen(w http.ResponseWriter, r *http.Request, username, id string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	routes, err := s.Routes.Routes(r.Context(), username)
	if err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	for _, route := range routes {
		if route.ID == id {
			var seen *time.Time
			if !route.LastSeenValue.IsZero() {
				v := route.LastSeenValue
				seen = &v
			}
			writeJSON(w, http.StatusOK, LastSeenResponse{RouteID: id, LastSeen: seen})
			return
		}
	}
	writeError(w, http.StatusNotFound, state.ErrNoRoute.Error())
}

func (s Server) enableDisable(w http.ResponseWriter, r *http.Request, username string, enabled bool) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.Routes.SetEnabled(r.Context(), username, enabled); err != nil {
		writeError(w, statusFor(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s Server) authorize(r *http.Request, username string) error {
	if s.Peers == nil {
		return nil
	}
	cred, err := s.Peers.PeerCredentials(r)
	if err != nil {
		return err
	}
	return authorizeUser(r.Context(), s.Store, username, cred)
}

func (s Server) requireRoot(r *http.Request) error {
	if s.Peers == nil {
		return nil
	}
	cred, err := s.Peers.PeerCredentials(r)
	if err != nil {
		return err
	}
	if cred.UID != 0 {
		return errForbidden
	}
	return nil
}

func routeResponse(r state.Route) RouteResponse {
	var seen *time.Time
	if !r.LastSeenValue.IsZero() {
		v := r.LastSeenValue
		seen = &v
	}
	return RouteResponse{RouteID: r.ID, Username: r.Username, Kind: string(r.Kind), PluginID: r.PluginID, Hostname: r.Hostname, URL: "https://" + r.Hostname, LocalPort: r.LocalPort, Enabled: r.Enabled, LastSeen: seen}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, ErrorResponse{Error: msg})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, errBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, errForbidden):
		return http.StatusForbidden
	case errors.Is(err, state.ErrNoUser), errors.Is(err, state.ErrNoRoute):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
