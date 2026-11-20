package server

import (
	"net/http"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// updateRoutes add the scheduled updates of the flake inputs, in Inputs.
// Running one pushes a branch and merging one deploys it, so both need a full
// token; the schedule is a setting, for the web login only.
func (s *Server) updateRoutes(api *http.ServeMux) {
	updates := func() (*core.Updates, error) {
		if s.Svc.Updates == nil {
			return nil, &core.Error{Kind: core.ErrNotFound, Msg: "scheduled updates are not set up"}
		}
		return s.Svc.Updates, nil
	}
	api.HandleFunc("GET /api/updates", get(func(r *http.Request) (any, error) {
		u, err := updates()
		if err != nil {
			return nil, err
		}
		return u.Status(r.Context()), nil
	}))
	api.HandleFunc("POST /api/updates/schedule", post(func(r *http.Request, in core.UpdateSchedule) (any, error) {
		u, err := updates()
		if err != nil {
			return nil, err
		}
		return u.SetSchedule(in)
	}))
	api.HandleFunc("POST /api/updates/run", post(func(r *http.Request, _ struct{}) (any, error) {
		u, err := updates()
		if err != nil {
			return nil, err
		}
		return u.Start(r.Context())
	}))
	api.HandleFunc("POST /api/updates/merge", post(func(r *http.Request, in struct{ Commit string }) (any, error) {
		u, err := updates()
		if err != nil {
			return nil, err
		}
		msg, err := u.Merge(r.Context(), in.Commit)
		return map[string]string{"message": msg}, err
	}))
	api.HandleFunc("POST /api/updates/discard", post(func(r *http.Request, _ struct{}) (any, error) {
		u, err := updates()
		if err != nil {
			return nil, err
		}
		return u.Discard()
	}))
}
