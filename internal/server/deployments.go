package server

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
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
	api.HandleFunc("GET /api/hosts/{host}/dependencies", get(func(r *http.Request) (any, error) {
		return s.Svc.Deploys.MachineDependencies(r.PathValue("host"))
	}))
	api.HandleFunc("GET /api/dependencies", get(func(r *http.Request) (any, error) {
		m, _, _ := s.Svc.Jobs.Model(r.Context(), "working") // nil while it is evaluated
		return s.Svc.Deploys.FleetDependencies(m), nil
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
	// Older report scripts only look at the status, so the question costs them nothing.
	if !in.SharedToken && s.Svc.Deploys.WantsClosure(in.Host, in.Current) {
		respondCode(w, http.StatusOK, map[string]string{"closure": "wanted"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deployClosure is what a machine posts when a report's reply asked for the
// closure of the system it runs: no session, the machine's own report token
// (never the shared fleet token, with which any machine could write
// another's), checked before the body is read. Host and system come in the
// query; a body too large is refused and the refusal kept.
func (s *Server) deployClosure(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	host, system := q.Get("host"), q.Get("system")
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	shared := false
	if ok && s.Svc.Deploys != nil {
		shared, ok = s.Svc.Deploys.Authorize(strings.TrimSpace(secret), host)
	}
	if !ok || s.Svc.Deploys == nil {
		slog.Warn("closure with a wrong token", "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "a valid report token for this machine is required (Deployments shows each machine's)")
		return
	}
	if shared {
		writeError(w, http.StatusForbidden, "a closure needs this machine's own report token, not the shared fleet token")
		return
	}
	var in core.ClosureIn
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, core.MaxClosureBody)).Decode(&in); err != nil {
		var tooBig *http.MaxBytesError
		if !errors.As(err, &tooBig) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		limit := fmt.Sprintf("%d bytes", core.MaxClosureBody)
		if core.MaxClosureBody%(1<<20) == 0 {
			limit = fmt.Sprintf("%d MiB", core.MaxClosureBody>>20)
		}
		if err := s.Svc.Deploys.RefuseClosure(host, system, "it is larger than "+limit, time.Now()); err != nil && !errors.Is(err, core.ErrInvalid) {
			slog.Warn("cannot keep a refused closure", "host", host, "err", err)
		}
		slog.Warn("closure refused: too large", "host", host)
		writeError(w, http.StatusRequestEntityTooLarge, "the closure is larger than "+limit)
		return
	}
	if err := s.Svc.Deploys.RecordClosure(host, system, in, time.Now()); err != nil {
		slog.Warn("closure not kept", "host", host, "err", err)
		respond(w, nil, err)
		return
	}
	if in.Error != "" {
		slog.Warn("a machine could not read its closure", "host", host, "err", in.Error)
	} else {
		slog.Info("closure received", "host", host, "paths", len(in.Paths))
	}
	w.WriteHeader(http.StatusNoContent)
}
