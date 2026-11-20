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

// deploymentRoutes are the session routes of the Deployments view.
func (s *Server) deploymentRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/deployments", get(func(r *http.Request) (any, error) {
		m, _, _ := s.Svc.Jobs.Model(r.Context(), "working") // nil while it is evaluated
		return s.Svc.Deploys.Overview(r.Context(), m, time.Now())
	}))
	api.HandleFunc("GET /api/deployments/token", get(func(r *http.Request) (any, error) {
		return map[string]string{"token": s.Svc.Deploys.Token()}, nil
	}))
	api.HandleFunc("POST /api/deployments/token", post(func(r *http.Request, _ struct{}) (any, error) {
		t, err := s.Svc.Deploys.RegenerateToken()
		return map[string]string{"token": t}, err
	}))
}

// deployReport is what a machine calls after comin deployed (no session: the
// fleet report token, compared in constant time, and a small capped body).
func (s *Server) deployReport(w http.ResponseWriter, r *http.Request) {
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || !s.Svc.Deploys.CheckToken(strings.TrimSpace(secret)) {
		slog.Warn("deployment report with a wrong token", "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "a valid deployment report token is required (Deployments shows it)")
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
	if err := s.Svc.Deploys.Record(in, time.Now()); err != nil {
		respond(w, nil, err)
		return
	}
	slog.Info("deployment reported", "host", in.Host, "commit", in.Commit[:12], "phase", cmp.Or(in.Phase, "deploy"), "status", in.Status)
	w.WriteHeader(http.StatusNoContent)
}
