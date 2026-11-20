package server

import (
	"cmp"
	"errors"
	"io"
	"net/http"
)

const maxHookBody = 1 << 20

// forgejoRoutes adds Settings → Forgejo. Every call here needs a session.
func (s *Server) forgejoRoutes(api *http.ServeMux) {
	f := s.Forgejo
	if f == nil {
		return
	}
	state := func(r *http.Request) (any, error) { return f.State(r.Context()) }
	api.HandleFunc("GET /api/forgejo", get(state))
	api.HandleFunc("POST /api/forgejo/remote", post(func(r *http.Request, in struct{ URL string }) (any, error) {
		if err := s.Svc.SetRemote(r.Context(), in.URL); err != nil {
			return nil, err
		}
		return state(r)
	}))
	api.HandleFunc("POST /api/forgejo/web", post(func(r *http.Request, in struct{ URL string }) (any, error) {
		if err := s.Svc.SetWebBase(r.Context(), in.URL); err != nil {
			return nil, err
		}
		return state(r)
	}))
	api.HandleFunc("POST /api/forgejo/key", post(func(r *http.Request, _ struct{}) (any, error) {
		if _, err := f.Key(true); err != nil {
			return nil, err
		}
		return state(r)
	}))
	api.HandleFunc("POST /api/forgejo/test", post(func(r *http.Request, _ struct{}) (any, error) {
		return f.Test(r.Context()), nil
	}))
	api.HandleFunc("POST /api/forgejo/status", post(func(r *http.Request, in struct{ Token, Console string }) (any, error) {
		if err := f.SetStatusToken(r.Context(), in.Token, in.Console); err != nil {
			return nil, err
		}
		return state(r)
	}))
	api.HandleFunc("POST /api/forgejo/adopt", post(func(r *http.Request, _ struct{}) (any, error) {
		out, err := s.Svc.AdoptRemote(r.Context())
		return map[string]string{"output": out}, err
	}))
}

// forgejoHook is Forgejo's push webhook. It has no session: the HMAC signature
// over the size-capped body is the authentication, and the body never chooses
// what git runs.
func (s *Server) forgejoHook(w http.ResponseWriter, r *http.Request) {
	if s.Forgejo == nil {
		writeError(w, http.StatusNotFound, "the webhook is not enabled")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHookBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "the body is larger than 1 MB")
		} else {
			writeError(w, http.StatusBadRequest, "cannot read the body")
		}
		return
	}
	sig := cmp.Or(r.Header.Get("X-Forgejo-Signature"), r.Header.Get("X-Gitea-Signature"))
	event := cmp.Or(r.Header.Get("X-Forgejo-Event"), r.Header.Get("X-Gitea-Event"))
	code, msg := s.Forgejo.Hook(event, sig, body)
	respondCode(w, code, map[string]string{"result": msg})
}
