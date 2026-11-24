package server

import (
	"net/http"
	"os"

	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

// isoRoutes add the installer ISOs, for the web UI login only: a build takes
// the server's time and disk, and an ISO holds the whole repository.
func (s *Server) isoRoutes(api *http.ServeMux) {
	isos := s.Svc.ISOs
	if isos == nil {
		return
	}
	api.HandleFunc("GET /api/isos", get(func(r *http.Request) (any, error) {
		list, err := isos.List()
		return map[string]any{"isos": list}, err
	}))
	api.HandleFunc("POST /api/isos", post(func(r *http.Request, in struct{ Host string }) (any, error) {
		return isos.Build(r.Context(), in.Host)
	}))
	api.HandleFunc("GET /api/isos/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		path, iso, err := isos.File(r.PathValue("id"))
		if err != nil {
			respond(w, nil, err)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			respond(w, nil, &core.Error{Kind: core.ErrNotFound, Msg: "the ISO file is gone"})
			return
		}
		defer f.Close()
		h := w.Header()
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", `attachment; filename="`+iso.File+`"`) // nixos-<machine name>-<hex>.iso: nothing to escape
		h.Set("X-Checksum-Sha256", iso.SHA256)
		h.Set("Cache-Control", "private, max-age=0")
		http.ServeContent(w, r, iso.File, iso.Finished, f) // ranges, so a broken download resumes
	})
}
