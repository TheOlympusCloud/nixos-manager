package server

import (
	"net/http"
)

// secretRoutes add Settings → Secrets. Names and keys are for any reader;
// values go in with edit access and come out only with full access, when asked
// (tokenRoutes).
func (s *Server) secretRoutes(api *http.ServeMux) {
	sec := s.Svc.Secrets
	if sec == nil {
		return
	}
	type secretIn struct{ Host, Name, Value string }
	api.HandleFunc("GET /api/secrets", get(func(r *http.Request) (any, error) { return sec.Status(r.Context()) }))
	api.HandleFunc("PUT /api/secrets/machines/{host}", post(func(r *http.Request, in struct{ Key string }) (any, error) {
		return sec.SetMachineKey(r.Context(), r.PathValue("host"), in.Key)
	}))
	api.HandleFunc("DELETE /api/secrets/machines/{host}", post(func(r *http.Request, _ struct{}) (any, error) {
		return nil, sec.RemoveMachineKey(r.Context(), r.PathValue("host"))
	}))
	api.HandleFunc("PUT /api/secrets", post(func(r *http.Request, in secretIn) (any, error) {
		return nil, sec.Set(r.Context(), in.Host, in.Name, in.Value)
	}))
	api.HandleFunc("POST /api/secrets/delete", post(func(r *http.Request, in secretIn) (any, error) {
		return nil, sec.Delete(r.Context(), in.Host, in.Name)
	}))
	reveal := post(func(r *http.Request, in secretIn) (any, error) {
		v, err := sec.Reveal(r.Context(), in.Host, in.Name)
		return map[string]string{"value": v}, err
	})
	api.HandleFunc("POST /api/secrets/reveal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store") // a value: kept by no browser or proxy
		reveal(w, r)
	})
	api.HandleFunc("POST /api/secrets/reencrypt", post(func(r *http.Request, _ struct{}) (any, error) {
		return nil, sec.Reencrypt(r.Context())
	}))
	api.HandleFunc("POST /api/secrets/report-tokens", post(func(r *http.Request, in struct{ Hosts []string }) (any, error) {
		hosts, err := s.Svc.StoreReportTokens(r.Context(), in.Hosts...)
		return map[string][]string{"hosts": hosts}, err
	}))
}
