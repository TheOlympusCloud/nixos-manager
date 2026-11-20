package server

import (
	"cmp"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// deploymentRoutes are the routes of the Deployments view.
func (s *Server) deploymentRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/deployments", get(func(r *http.Request) (any, error) {
		m, _, _ := s.Svc.Jobs.Model(r.Context(), "working") // nil while it is evaluated
		return s.Svc.Deploys.Overview(r.Context(), m, time.Now())
	}))
	api.HandleFunc("GET /api/deployments/token", get(func(r *http.Request) (any, error) {
		t, err := s.Svc.Deploys.HostToken(r.URL.Query().Get("host"))
		return map[string]string{"token": t}, err
	}))
	api.HandleFunc("POST /api/deployments/token", post(func(r *http.Request, _ struct{}) (any, error) {
		hosts, err := s.Svc.RotateReportTokens(r.Context())
		return map[string][]string{"stored": hosts}, err
	}))
	api.HandleFunc("DELETE /api/deployments/shared-token", post(func(r *http.Request, _ struct{}) (any, error) {
		return nil, s.Svc.Deploys.DropSharedToken()
	}))
	api.HandleFunc("DELETE /api/deployments/machines/{host}", post(func(r *http.Request, _ struct{}) (any, error) {
		return nil, s.Svc.Deploys.Forget(r.PathValue("host"))
	}))
}

// deployReport is what a machine calls after comin deployed and for its
// heartbeat (no session: the machine's own report token, compared in constant
// time with the one derived for the host the body names, and a small capped
// body).
func (s *Server) deployReport(w http.ResponseWriter, r *http.Request) {
	unauthorized := func() {
		slog.Warn("deployment report with a wrong token", "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "a valid report token for this machine is required (Deployments shows each machine's)")
	}
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || s.Svc.Deploys == nil {
		unauthorized()
		return
	}
	var in core.ReportIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, core.MaxReportBody)).Decode(&in); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "the report is too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}
	if in.SharedToken, ok = s.Svc.Deploys.Authorize(strings.TrimSpace(secret), in.Host); !ok {
		unauthorized()
		return
	}
	if err := s.Svc.Deploys.Record(in, time.Now()); err != nil {
		respond(w, nil, err)
		return
	}
	if in.Phase == "heartbeat" { // every 15 minutes from every machine: not worth a line each
		slog.Debug("heartbeat", "host", in.Host)
	} else {
		slog.Info("deployment reported", "host", in.Host, "commit", in.Commit[:12], "phase", cmp.Or(in.Phase, "deploy"), "status", in.Status)
	}
	w.WriteHeader(http.StatusNoContent)
}
