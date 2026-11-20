// Package server is the HTTP side: the JSON API for the web UI, the editor's
// language-server socket, the MCP endpoint and the built UI.
package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/TheOlympusCloud/nixos-manager/internal/auth"
	"github.com/TheOlympusCloud/nixos-manager/internal/core"
)

const (
	cookieName    = "nixmgr_session"
	browserCookie = "nixmgr_browser" // marks a browser that logged in before, for the login limits
	maxBodySize   = 32 << 20
)

type Server struct {
	Svc  *core.Service
	Auth *auth.Auth
	UI   fs.FS        // the built web UI; index.html at its root
	MCP  http.Handler // MCP endpoint, guarded here by bearer tokens
	Nixd []string     // language server command

	Forgejo *core.Forgejo  // Settings → Forgejo and the push webhook; nil turns them off
	Notify  *core.Notifier // Settings → Notifications; nil turns them off

	// TrustedProxies are reverse proxies, besides loopback, whose X-Forwarded-For
	// names the client for the login limits (e.g. the Docker gateway).
	TrustedProxies []netip.Prefix

	lspSlots chan struct{}
	gz       sync.Map // static path -> gzipped bytes
}

// Handler wires every route. Everything except the login call, the static UI
// and the machines' and Forgejo's own calls needs the web UI login or an API
// token whose scope covers the route.
func (s *Server) Handler() http.Handler {
	s.lspSlots = make(chan struct{}, maxNixd)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.login)
	// Machines report what comin deployed with the fleet report token, without a session.
	mux.HandleFunc("POST /api/deployments/report", s.deployReport)
	mux.HandleFunc("POST /api/hooks/forgejo", s.forgejoHook) // signed by Forgejo, no session
	mux.Handle("/api/", s.requireAuth(s.apiMux()))
	if s.MCP != nil {
		// The token's id binds each MCP session to the token that opened it.
		mux.Handle("/mcp", mcpauth.RequireBearerToken(s.mcpToken, &mcpauth.RequireBearerTokenOptions{AllowMissingExpiration: true})(s.MCP))
	}
	mux.Handle("/", s.static())
	return headers(http.NewCrossOriginProtection().Handler(jsonOnly(mux)))
}

// tokenRoutes are the API routes an API token may call, with the scope each
// needs. Every other route (tokens, Settings, the report token and storing the
// report tokens as secrets, the editor's language server) needs the web UI login.
var tokenRoutes = map[string]auth.Scope{
	"/api/": auth.ScopeRead, // no such route: 404

	"GET /api/info": auth.ScopeRead, "GET /api/tree": auth.ScopeRead, "GET /api/file": auth.ScopeRead,
	"POST /api/format": auth.ScopeRead, "GET /api/status": auth.ScopeRead, "GET /api/diff": auth.ScopeRead,
	"GET /api/log": auth.ScopeRead, "GET /api/commit": auth.ScopeRead, "GET /api/file-at": auth.ScopeRead,
	"GET /api/branches": auth.ScopeRead, "GET /api/hosts/{host}/managed": auth.ScopeRead,
	"GET /api/search/packages": auth.ScopeRead, "GET /api/search/options": auth.ScopeRead,
	"POST /api/search/refresh": auth.ScopeRead, "GET /api/insights": auth.ScopeRead,
	"GET /api/fleet/values": auth.ScopeRead, "GET /api/model": auth.ScopeRead, "POST /api/model/refresh": auth.ScopeRead,
	"GET /api/model/diff": auth.ScopeRead, "GET /api/targets": auth.ScopeRead, "GET /api/jobs": auth.ScopeRead,
	"GET /api/jobs/{id}": auth.ScopeRead, "GET /api/jobs/{id}/events": auth.ScopeRead, "GET /api/deployments": auth.ScopeRead,
	"GET /api/secrets": auth.ScopeRead, // names and keys, never a value

	"PUT /api/file": auth.ScopeEdit, "POST /api/mkdir": auth.ScopeEdit, "POST /api/move": auth.ScopeEdit,
	"POST /api/delete": auth.ScopeEdit, "POST /api/stage": auth.ScopeEdit, "POST /api/unstage": auth.ScopeEdit,
	"POST /api/discard": auth.ScopeEdit, "POST /api/commit": auth.ScopeEdit, "POST /api/restore": auth.ScopeEdit,
	"POST /api/branches": auth.ScopeEdit, "POST /api/switch": auth.ScopeEdit, "POST /api/revert": auth.ScopeEdit,
	"POST /api/merge": auth.ScopeEdit, "POST /api/pull": auth.ScopeEdit, "POST /api/hosts/{host}/packages": auth.ScopeEdit,
	"POST /api/hosts/{host}/settings": auth.ScopeEdit, "POST /api/machines": auth.ScopeEdit, "POST /api/jobs": auth.ScopeEdit,
	"PUT /api/secrets": auth.ScopeEdit, "POST /api/secrets/delete": auth.ScopeEdit, "POST /api/secrets/reencrypt": auth.ScopeEdit,
	"PUT /api/secrets/machines/{host}": auth.ScopeEdit, "DELETE /api/secrets/machines/{host}": auth.ScopeEdit,

	"POST /api/push": auth.ScopeFull, // a push is a deploy
	"POST /api/try":  auth.ScopeFull, "POST /api/try/end": auth.ScopeFull,
	"POST /api/secrets/reveal": auth.ScopeFull, // a value, in plain text
}

func (s *Server) apiMux() *http.ServeMux {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/logout", s.logout)
	api.HandleFunc("GET /api/info", get(s.info))
	api.HandleFunc("GET /api/tree", get(func(r *http.Request) (any, error) {
		entries, err := s.Svc.Tree()
		return map[string]any{"entries": entries}, err
	}))
	api.HandleFunc("GET /api/file", get(func(r *http.Request) (any, error) {
		p := r.URL.Query().Get("path")
		content, hash, err := s.Svc.Read(p)
		return map[string]string{"path": p, "content": content, "hash": hash}, err
	}))
	api.HandleFunc("PUT /api/file", post(func(r *http.Request, in struct {
		Path, Content string
		BaseHash      string `json:"base_hash"`
	}) (any, error) {
		hash, err := s.Svc.Write(in.Path, in.Content, in.BaseHash)
		return map[string]string{"hash": hash}, err
	}))
	api.HandleFunc("POST /api/mkdir", post(func(r *http.Request, in struct{ Path string }) (any, error) {
		return nil, s.Svc.Mkdir(in.Path)
	}))
	api.HandleFunc("POST /api/move", post(func(r *http.Request, in struct{ From, To string }) (any, error) {
		return nil, s.Svc.Move(in.From, in.To)
	}))
	api.HandleFunc("POST /api/delete", post(func(r *http.Request, in struct{ Path string }) (any, error) {
		return nil, s.Svc.Delete(in.Path)
	}))
	api.HandleFunc("POST /api/format", post(func(r *http.Request, in struct{ Content string }) (any, error) {
		out, err := s.Svc.Jobs.Format(r.Context(), in.Content)
		return map[string]string{"content": out}, err
	}))

	api.HandleFunc("GET /api/status", get(func(r *http.Request) (any, error) { return s.Svc.Status(r.Context()) }))
	api.HandleFunc("GET /api/diff", get(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		d, err := s.Svc.Diff(r.Context(), q.Get("path"), q.Get("staged") == "true")
		return map[string]string{"diff": d}, err
	}))
	api.HandleFunc("POST /api/stage", post(func(r *http.Request, in struct{ Paths []string }) (any, error) {
		return nil, s.Svc.Stage(r.Context(), in.Paths)
	}))
	api.HandleFunc("POST /api/unstage", post(func(r *http.Request, in struct{ Paths []string }) (any, error) {
		return nil, s.Svc.Unstage(r.Context(), in.Paths)
	}))
	api.HandleFunc("POST /api/discard", post(func(r *http.Request, in struct{ Paths []string }) (any, error) {
		return nil, s.Svc.Discard(r.Context(), in.Paths)
	}))
	api.HandleFunc("POST /api/commit", post(func(r *http.Request, in struct {
		Message string
		Force   bool
	}) (any, error) {
		sha, err := s.Svc.Commit(r.Context(), in.Message, in.Force)
		return map[string]string{"sha": sha}, err
	}))
	api.HandleFunc("GET /api/log", get(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		commits, err := s.Svc.Log(r.Context(), q.Get("path"), q.Get("rev"), limit)
		return map[string]any{"commits": commits}, err
	}))
	api.HandleFunc("GET /api/commit", get(func(r *http.Request) (any, error) {
		return s.Svc.CommitInfo(r.Context(), r.URL.Query().Get("rev"))
	}))
	api.HandleFunc("GET /api/file-at", get(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		content, err := s.Svc.FileAt(r.Context(), q.Get("rev"), q.Get("path"))
		return map[string]string{"content": content}, err
	}))
	api.HandleFunc("POST /api/restore", post(func(r *http.Request, in struct {
		Path, Rev string
		BaseHash  string `json:"base_hash"`
	}) (any, error) {
		return nil, s.Svc.Restore(r.Context(), in.Path, in.Rev, in.BaseHash)
	}))
	api.HandleFunc("GET /api/branches", get(func(r *http.Request) (any, error) { return s.Svc.Branches(r.Context()) }))
	api.HandleFunc("POST /api/branches", post(func(r *http.Request, in struct{ Name, From string }) (any, error) {
		return nil, s.Svc.CreateBranch(r.Context(), in.Name, in.From)
	}))
	api.HandleFunc("POST /api/switch", post(func(r *http.Request, in struct{ Name string }) (any, error) {
		return nil, s.Svc.Switch(r.Context(), in.Name)
	}))
	api.HandleFunc("POST /api/revert", post(func(r *http.Request, in struct{ Rev, Since string }) (any, error) {
		return nil, s.Svc.Revert(r.Context(), in.Rev, in.Since)
	}))
	api.HandleFunc("POST /api/merge", post(func(r *http.Request, in struct{ Branch string }) (any, error) {
		msg, err := s.Svc.Merge(r.Context(), in.Branch)
		return map[string]string{"message": msg}, err
	}))
	api.HandleFunc("POST /api/try", post(func(r *http.Request, in struct {
		Host, Message string
		Replace       bool
	}) (any, error) {
		return s.Svc.TryOn(r.Context(), in.Host, in.Message, in.Replace)
	}))
	api.HandleFunc("POST /api/try/end", post(func(r *http.Request, in struct{ Host string }) (any, error) {
		msg, err := s.Svc.EndTrial(r.Context(), in.Host)
		return map[string]string{"message": msg}, err
	}))
	api.HandleFunc("POST /api/push", post(func(r *http.Request, in struct{ Force bool }) (any, error) {
		out, err := s.Svc.Push(r.Context(), in.Force)
		return map[string]string{"output": out}, err
	}))
	api.HandleFunc("POST /api/pull", post(func(r *http.Request, _ struct{}) (any, error) {
		out, err := s.Svc.Pull(r.Context())
		return map[string]string{"output": out}, err
	}))

	api.HandleFunc("GET /api/hosts/{host}/managed", get(func(r *http.Request) (any, error) {
		return s.Svc.ManagedView(r.PathValue("host"))
	}))
	api.HandleFunc("POST /api/hosts/{host}/packages", post(func(r *http.Request, in struct{ Add, Remove []string }) (any, error) {
		host := r.PathValue("host")
		v, err := s.Svc.ManagedView(host)
		if err == nil && len(in.Add) > 0 {
			v, err = s.Svc.AddPackages(host, in.Add)
		}
		if err == nil && len(in.Remove) > 0 {
			v, err = s.Svc.RemovePackages(host, in.Remove)
		}
		return v, err
	}))
	api.HandleFunc("POST /api/hosts/{host}/settings", post(func(r *http.Request, in struct {
		Path     []string
		Value    any
		Unset    bool
		BaseHash *string `json:"base_hash"` // the managed file the edit is based on; 409 if it changed
	}) (any, error) {
		if in.Unset {
			return s.Svc.UnsetOption(r.PathValue("host"), in.Path, in.BaseHash)
		}
		return s.Svc.SetOption(r.PathValue("host"), in.Path, in.Value, in.BaseHash)
	}))
	api.HandleFunc("GET /api/search/packages", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		res, j, err := s.Svc.Jobs.SearchPackages(r.Context(), q.Get("q"), limit)
		indexed(w, res, j, err)
	})
	api.HandleFunc("GET /api/search/options", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("exact") == "1" {
			o, j, err := s.Svc.Jobs.Option(r.Context(), q.Get("host"), q.Get("q"))
			if o != nil {
				indexed(w, []core.OptionDoc{*o}, nil, nil)
			} else {
				indexed[core.OptionDoc](w, nil, j, err)
			}
			return
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		res, j, err := s.Svc.Jobs.SearchOptions(r.Context(), q.Get("host"), q.Get("q"), limit)
		indexed(w, res, j, err)
	})
	api.HandleFunc("POST /api/search/refresh", post(func(r *http.Request, in struct{ Kind, Host string }) (any, error) {
		j, err := s.Svc.Jobs.RefreshIndex(r.Context(), in.Kind, in.Host)
		if err != nil {
			return nil, err
		}
		return map[string]string{"job": j.ID}, nil
	}))
	api.HandleFunc("GET /api/insights", func(w http.ResponseWriter, r *http.Request) {
		m, j, err := s.Svc.Jobs.Model(r.Context(), "working")
		switch {
		case err != nil:
			respond(w, nil, err)
		case m == nil:
			respondCode(w, http.StatusAccepted, map[string]string{"state": "computing", "job": j.ID})
		default:
			// Deployment findings are extra: Health still answers when git cannot tell them.
			ov, _ := s.Svc.Deploys.Overview(r.Context(), m, time.Now())
			tip := s.Svc.TipCheck(r.Context(), ov.Branch)
			respond(w, map[string]any{"state": "ready", "insights": core.Insights(m, ov.Machines, s.Svc.Remote(r.Context()), tip, time.Now())}, nil)
		}
	})
	api.HandleFunc("GET /api/fleet/values", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res, j, err := s.Svc.Jobs.OptionValues(r.Context(), q["name"], q["host"])
		switch {
		case err != nil:
			respond(w, nil, err)
		case j != nil:
			respondCode(w, http.StatusAccepted, map[string]string{"state": "computing", "job": j.ID})
		default:
			respond(w, map[string]any{"state": "ready", "values": res}, nil)
		}
	})
	api.HandleFunc("POST /api/machines", post(func(r *http.Request, in core.MachineSpec) (any, error) {
		return s.Svc.CreateMachine(in)
	}))
	api.HandleFunc("GET /api/model", s.model)
	api.HandleFunc("POST /api/model/refresh", post(func(r *http.Request, _ struct{}) (any, error) {
		j, err := s.Svc.Jobs.RefreshModel(r.Context())
		if err != nil {
			return nil, err
		}
		return map[string]string{"job": j.ID}, nil
	}))
	api.HandleFunc("GET /api/model/diff", s.modelDiff)
	api.HandleFunc("GET /api/targets", get(func(r *http.Request) (any, error) {
		t, err := s.Svc.Jobs.Targets(r.Context())
		return map[string]any{"targets": t}, err
	}))
	api.HandleFunc("POST /api/jobs", post(func(r *http.Request, in core.JobRequest) (any, error) {
		j, err := s.Svc.Jobs.Submit(r.Context(), in)
		if err != nil {
			return nil, err
		}
		return map[string]string{"id": j.ID}, nil
	}))
	api.HandleFunc("GET /api/jobs", get(func(r *http.Request) (any, error) {
		return map[string]any{"jobs": s.Svc.Jobs.List()}, nil
	}))
	api.HandleFunc("GET /api/jobs/{id}", get(func(r *http.Request) (any, error) {
		j, ok := s.Svc.Jobs.Get(r.PathValue("id"))
		if !ok {
			return nil, &core.Error{Kind: core.ErrNotFound, Msg: "no such job"}
		}
		return j.Info(true), nil
	}))
	api.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)

	api.HandleFunc("GET /api/tokens", get(func(r *http.Request) (any, error) {
		return map[string]any{"tokens": s.Auth.Tokens()}, nil
	}))
	api.HandleFunc("POST /api/tokens", post(func(r *http.Request, in struct {
		Name    string
		Scope   auth.Scope
		Expires time.Time // zero: never
	}) (any, error) {
		t, secret, err := s.Auth.CreateToken(in.Name, in.Scope, in.Expires)
		if err != nil {
			return nil, &core.Error{Kind: core.ErrInvalid, Msg: err.Error()}
		}
		return struct {
			auth.TokenInfo
			Token string `json:"token"`
		}{t, secret}, nil
	}))
	api.HandleFunc("DELETE /api/tokens/{id}", post(func(r *http.Request, _ struct{}) (any, error) {
		if err := s.Auth.RevokeToken(r.PathValue("id")); errors.Is(err, auth.ErrNoToken) {
			return nil, &core.Error{Kind: core.ErrNotFound, Msg: "no such token"}
		} else {
			return nil, err
		}
	}))
	api.HandleFunc("GET /api/lsp", s.lsp)
	s.forgejoRoutes(api)
	s.deploymentRoutes(api)
	s.notifyRoutes(api)
	s.secretRoutes(api)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such API route")
	})
	return api
}

func headers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// jsonOnly rejects state-changing API calls that are not JSON, which a
// cross-site form cannot send.
func jsonOnly(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead {
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "send JSON with Content-Type: application/json")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// requireAuth lets in an API token on the routes its scope covers, or else
// the web UI's session, and tells the service who is asking. A request that
// carries a token is judged by the token alone.
func (s *Server) requireAuth(api *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); h != "" {
			secret, ok := strings.CutPrefix(h, "Bearer ")
			t, valid := s.Auth.VerifyToken(strings.TrimSpace(secret))
			if !ok || !valid {
				w.Header().Set("WWW-Authenticate", `Bearer realm="nixos-manager"`)
				writeError(w, http.StatusUnauthorized, "a valid API token is required (create one on the Tokens page)")
				return
			}
			_, pattern := api.Handler(r)
			need, listed := tokenRoutes[pattern]
			if !listed {
				writeError(w, http.StatusForbidden, "API tokens cannot use this route; it needs the web UI login")
				return
			}
			if !t.Scope.Allows(need) {
				writeError(w, http.StatusForbidden, fmt.Sprintf("this token's scope is %s; this route needs %s", t.Scope, need))
				return
			}
			api.ServeHTTP(w, r.WithContext(core.WithCaller(r.Context(), core.Caller{Via: t.Via()})))
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.Auth.Session(c.Value) {
			writeError(w, http.StatusUnauthorized, "log in first")
			return
		}
		via := "web session " + auth.SessionLabel(c.Value)
		api.ServeHTTP(w, r.WithContext(core.WithCaller(r.Context(), core.Caller{Via: via, Person: true})))
	})
}

// mcpToken checks the bearer token of an MCP request; the MCP server picks
// its tools by the scope and names the token in commits.
func (s *Server) mcpToken(_ context.Context, secret string, _ *http.Request) (*mcpauth.TokenInfo, error) {
	t, ok := s.Auth.VerifyToken(secret)
	if !ok {
		return nil, fmt.Errorf("%w: a valid API token is required (create one on the Tokens page)", mcpauth.ErrInvalidToken)
	}
	return &mcpauth.TokenInfo{Scopes: []string{string(t.Scope)}, Expiration: t.Expires, UserID: t.ID, Extra: map[string]any{"via": t.Via()}}, nil
}

func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Password string }
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	source, known := "ip "+s.clientIP(r), false
	if c, err := r.Cookie(browserCookie); err == nil {
		if id, ok := s.Auth.CheckKnownBrowser(c.Value); ok {
			source, known = "browser "+id, true
		}
	}
	id, err := s.Auth.Login(in.Password, source, known)
	var limited *auth.LimitError
	if errors.As(err, &limited) { // not logged: refusals cost nothing and would flood the journal
		w.Header().Set("Retry-After", strconv.Itoa(int((limited.Wait+time.Second-1)/time.Second)))
		writeError(w, http.StatusTooManyRequests, limited.Error())
		return
	}
	if err != nil {
		slog.Warn("failed login", "remote", r.RemoteAddr, "source", source)
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	slog.Info("login", "session", auth.SessionLabel(id), "remote", r.RemoteAddr, "source", source)
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, Secure: secure(r),
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionTTL / time.Second)})
	http.SetCookie(w, &http.Cookie{Name: browserCookie, Value: s.Auth.KnownBrowser(), Path: "/api/login", HttpOnly: true,
		Secure: secure(r), SameSite: http.SameSiteStrictMode, MaxAge: 400 * 24 * 3600})
	w.WriteHeader(http.StatusNoContent)
}

// clientIP is the address the login limits count by: the peer, or behind a
// proxy on this machine (tailscale serve, Caddy, nginx) or a trusted one, the
// address the proxy appended to X-Forwarded-For. IPv6 counts by /64, which one
// host can fill.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip, perr := netip.ParseAddr(host)
	if err != nil || perr != nil {
		return r.RemoteAddr
	}
	proxy := ip.IsLoopback() || slices.ContainsFunc(s.TrustedProxies, func(p netip.Prefix) bool { return p.Contains(ip.Unmap()) })
	if xff := r.Header.Values("X-Forwarded-For"); proxy && len(xff) > 0 {
		last := xff[len(xff)-1]
		if fwd, err := netip.ParseAddr(strings.TrimSpace(last[strings.LastIndex(last, ",")+1:])); err == nil {
			ip = fwd
		}
	}
	if ip = ip.Unmap(); ip.Is6() {
		p, _ := ip.Prefix(64)
		return p.String()
	}
	return ip.String()
}

// ParseTrustedProxies reads NIXMGR_TRUSTED_PROXIES: addresses or CIDR ranges,
// separated by commas or spaces.
func ParseTrustedProxies(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		p, err := netip.ParsePrefix(f)
		if !strings.Contains(f, "/") {
			var a netip.Addr
			if a, err = netip.ParseAddr(f); err == nil {
				p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%q is not an address or CIDR range", f)
		}
		if p != p.Masked() {
			return nil, fmt.Errorf("%q has host bits set; did you mean %s?", f, p.Masked())
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.Auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) info(r *http.Request) (any, error) {
	st, err := s.Svc.Status(r.Context())
	return map[string]string{
		"root":   s.Svc.Repo.Dir,
		"branch": st.Branch,
		"author": fmt.Sprintf("%s <%s>", s.Svc.AuthorName, s.Svc.AuthorEmail),
		"remote": s.Svc.Remote(r.Context()),
		"web":    s.Svc.WebURL(r.Context()),
	}, err
}

// model answers 202 with the job while Nix evaluates the fleet, then 200.
func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	m, j, err := s.Svc.Jobs.Model(r.Context(), r.URL.Query().Get("at"))
	switch {
	case err != nil:
		respond(w, nil, err)
	case m == nil:
		respondCode(w, http.StatusAccepted, map[string]string{"state": "computing", "job": j.ID})
	default:
		respond(w, map[string]any{"state": "ready", "model": m}, nil)
	}
}

// indexed answers a search: 202 while the index is being built, else the results.
func indexed[T any](w http.ResponseWriter, results []T, j *core.Job, err error) {
	switch {
	case err != nil:
		respond(w, nil, err)
	case j != nil:
		respondCode(w, http.StatusAccepted, map[string]string{"state": "indexing", "job": j.ID})
	default:
		respond(w, map[string]any{"state": "ready", "results": results}, nil)
	}
}

// modelDiff compares the last commit with the working copy, machine by machine.
func (s *Server) modelDiff(w http.ResponseWriter, r *http.Request) {
	// Default: the last commit against the working copy. With ?rev=: that commit against its parent.
	// With ?against=pushed: what a push lands on against the last commit, which it deploys.
	from, to := "head", "working"
	switch against := r.URL.Query().Get("against"); {
	case against == "pushed":
		from, to = "pushed", "head"
	case against != "":
		writeError(w, http.StatusBadRequest, "against must be pushed")
		return
	case r.URL.Query().Get("rev") != "":
		rev := r.URL.Query().Get("rev")
		to = "rev:" + rev
		from = "" // a root commit is compared with nothing
		if parent, ok := s.Svc.Repo.Parent(r.Context(), rev); ok {
			from = "rev:" + parent
		}
	}
	model := func(at string) (*core.Model, *core.Job, error) {
		if at == "" {
			return &core.Model{}, nil, nil
		}
		return s.Svc.Jobs.Model(r.Context(), at)
	}
	head, hj, err := model(from)
	if err != nil {
		respond(w, nil, err)
		return
	}
	work, wj, err := model(to)
	if err != nil {
		respond(w, nil, err)
		return
	}
	if head == nil || work == nil {
		jobs := []string{}
		for _, j := range []*core.Job{hj, wj} {
			if j != nil {
				jobs = append(jobs, j.ID)
			}
		}
		respondCode(w, http.StatusAccepted, map[string]any{"state": "computing", "jobs": jobs})
		return
	}
	hosts := core.DiffModels(head, work)
	s.Svc.Jobs.AddClosures(r.Context(), head, work, hosts)
	out := map[string]any{"state": "ready", "hosts": hosts, "inputs": core.DiffInputs(head.Inputs, work.Inputs)}
	if work.Error != "" { // nothing to compare with: do not report every machine as removed
		out["hosts"], out["inputs"], out["error"] = []core.HostDiff{}, []core.InputChange{}, work.Error
	}
	if head.Error != "" { // every machine would look new; say why
		out["headError"] = head.Error
	}
	respond(w, out, nil)
}

// jobEvents streams a job's output as server-sent events, then a done event.
func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	j, ok := s.Svc.Jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such job")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	off := 0
	for {
		chunk, done, wait := j.Read(off)
		for !done && len(chunk) > 0 && !utf8.Valid(chunk) {
			chunk = chunk[:len(chunk)-1] // never split a UTF-8 character; the rest comes next round
		}
		if len(chunk) > 0 {
			b, _ := json.Marshal(string(chunk))
			fmt.Fprintf(w, "event: output\ndata: %s\n\n", b)
			off += len(chunk)
		}
		if done {
			b, _ := json.Marshal(map[string]string{"state": j.Info(false).State})
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", b)
			rc.Flush()
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
		select {
		case <-wait:
		case <-r.Context().Done():
			return
		}
	}
}

// static serves the built UI, falling back to index.html for client routes.
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" || fs.ValidPath(p) && !exists(s.UI, p) {
			index, err := fs.ReadFile(s.UI, "index.html")
			if err != nil {
				http.Error(w, "The web UI is not built. Run `npm run build` in web/.", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") { // Vite names them by content hash
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if gz := s.gzipped(p); gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.Write(gz)
			return
		}
		files.ServeHTTP(w, r)
	})
}

var compressible = map[string]bool{".js": true, ".css": true, ".svg": true, ".json": true, ".html": true, ".txt": true}

// gzipped returns a static file compressed once and kept in memory, or nil.
func (s *Server) gzipped(p string) []byte {
	if !compressible[path.Ext(p)] {
		return nil
	}
	if v, ok := s.gz.Load(p); ok {
		return v.([]byte)
	}
	raw, err := fs.ReadFile(s.UI, p)
	if err != nil || len(raw) < 512 && !strings.HasPrefix(p, "assets/") {
		return nil
	}
	var b bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	zw.Write(raw)
	zw.Close()
	s.gz.Store(p, b.Bytes())
	return b.Bytes()
}

func exists(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	return err == nil
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodySize)).Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %v", err)
	}
	return nil
}

func get(fn func(r *http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := fn(r)
		respond(w, out, err)
	}
}

// post decodes the JSON body into In; keys match field names case-insensitively.
func post[In any](fn func(r *http.Request, in In) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in In
		if err := decode(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		out, err := fn(r, in)
		respond(w, out, err)
	}
}

func respond(w http.ResponseWriter, out any, err error) {
	if err != nil {
		code := http.StatusInternalServerError
		for kind, c := range map[error]int{
			core.ErrInvalid: http.StatusBadRequest, core.ErrNothingStaged: http.StatusBadRequest,
			core.ErrNotFound: http.StatusNotFound, core.ErrExists: http.StatusConflict, core.ErrConflict: http.StatusConflict,
			core.ErrNotChecked: http.StatusPreconditionFailed, core.ErrSyntax: http.StatusUnprocessableEntity,
			core.ErrForbidden: http.StatusForbidden,
		} {
			if errors.Is(err, kind) {
				code = c
			}
		}
		if code == http.StatusInternalServerError {
			slog.Error("request failed", "err", err)
		}
		writeError(w, code, err.Error())
		return
	}
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func respondCode(w http.ResponseWriter, code int, out any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(out)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
