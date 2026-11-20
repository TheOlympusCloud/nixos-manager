package server

import (
	"net/http"
	"time"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// notifyRoutes add Settings → Notifications. Every call here needs a session.
func (s *Server) notifyRoutes(api *http.ServeMux) {
	off := &core.Error{Kind: core.ErrNotFound, Msg: "notifications are not set up"}
	api.HandleFunc("GET /api/notifications", get(func(r *http.Request) (any, error) {
		if s.Notify == nil {
			return nil, off
		}
		set, st := s.Notify.Settings()
		if st == nil {
			st = []core.ChannelStatus{}
		}
		return map[string]any{"settings": set, "status": st}, nil
	}))
	api.HandleFunc("POST /api/notifications", post(func(r *http.Request, in core.NotifySettings) (any, error) {
		if s.Notify == nil {
			return nil, off
		}
		set, err := s.Notify.Save(in, time.Now())
		return map[string]any{"settings": set}, err
	}))
	api.HandleFunc("POST /api/notifications/test", post(func(r *http.Request, _ struct{}) (any, error) {
		if s.Notify == nil {
			return nil, off
		}
		st, err := s.Notify.Test(r.Context(), time.Now())
		return map[string]any{"status": st}, err
	}))
}
