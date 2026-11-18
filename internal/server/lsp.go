package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// lsp bridges one browser WebSocket to one nixd process: each text message is
// one JSON-RPC message, framed with Content-Length on the nixd side. The bridge
// answers nixd's workspace/configuration requests itself, so completion and
// option docs come from this repository's flake whatever the editor sends.
func (s *Server) lsp(w http.ResponseWriter, r *http.Request) {
	opts := &websocket.AcceptOptions{} // same-origin only
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		opts.OriginPatterns = []string{fh} // behind a reverse proxy such as tailscale serve
	}
	c, err := websocket.Accept(w, r, opts)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(32 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, s.Nixd[0], s.Nixd[1:]...)
	cmd.Dir = s.Svc.Repo.Dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		slog.Error("starting the language server", "err", err)
		c.Close(websocket.StatusInternalError, "cannot start nixd")
		return
	}
	defer func() { cancel(); cmd.Wait() }() // kill nixd before reaping it

	var mu sync.Mutex
	send := func(msg []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if _, err := fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n", len(msg)); err != nil {
			return err
		}
		_, err := stdin.Write(msg)
		return err
	}
	settings := nixdSettings(s.Svc.Repo.Dir)

	go func() {
		defer cancel()
		br := bufio.NewReader(stdout)
		for {
			msg, err := readFrame(br)
			if err != nil {
				return
			}
			if reply := configReply(msg, settings); reply != nil {
				if send(reply) != nil {
					return
				}
				continue
			}
			if c.Write(ctx, websocket.MessageText, msg) != nil {
				return
			}
		}
	}()
	for {
		typ, msg, err := c.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageText && send(withConfigCapability(msg)) != nil {
			return
		}
	}
}

func readFrame(br *bufio.Reader) ([]byte, error) {
	n := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			if n, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
				return nil, err
			}
		}
	}
	if n < 0 || n > 64<<20 {
		return nil, errors.New("invalid LSP frame header")
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(br, buf)
	return buf, err
}

// withConfigCapability makes the initialize request advertise
// workspace/configuration, because nixd only pulls settings from clients that do.
func withConfigCapability(msg []byte) []byte {
	var m map[string]any
	if json.Unmarshal(msg, &m) != nil || m["method"] != "initialize" {
		return msg
	}
	params, _ := m["params"].(map[string]any)
	if params == nil {
		return msg
	}
	caps, _ := params["capabilities"].(map[string]any)
	if caps == nil {
		caps = map[string]any{}
		params["capabilities"] = caps
	}
	ws, _ := caps["workspace"].(map[string]any)
	if ws == nil {
		ws = map[string]any{}
		caps["workspace"] = ws
	}
	ws["configuration"] = true
	out, err := json.Marshal(m)
	if err != nil {
		return msg
	}
	return out
}

func configReply(msg []byte, settings map[string]any) []byte {
	var m struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Items []struct {
				Section string `json:"section"`
			} `json:"items"`
		} `json:"params"`
	}
	if json.Unmarshal(msg, &m) != nil || m.Method != "workspace/configuration" || m.ID == nil {
		return nil
	}
	result := make([]any, len(m.Params.Items))
	for i, it := range m.Params.Items {
		switch it.Section {
		case "nixd":
			result[i] = settings
		case "":
			result[i] = map[string]any{"nixd": settings}
		}
	}
	out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
	return out
}

func nixString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `${`, `\${`).Replace(s) + `"`
}

// nixdSettings points nixd at this repository's flake: nixpkgs from its inputs,
// option docs from its first NixOS, home-manager and nix-darwin configurations.
func nixdSettings(dir string) map[string]any {
	flake := "let f = builtins.getFlake " + nixString(dir) + "; first = a: builtins.head (builtins.attrValues a); in "
	return map[string]any{
		"nixpkgs":    map[string]any{"expr": flake + "import (if f.inputs ? nixpkgs then f.inputs.nixpkgs else <nixpkgs>) { }"},
		"formatting": map[string]any{"command": []string{"nixfmt"}},
		"options": map[string]any{
			"nixos": map[string]any{"expr": flake + "(first f.nixosConfigurations).options"},
			"home-manager": map[string]any{"expr": flake + "if f ? homeConfigurations && f.homeConfigurations != { } " +
				"then (first f.homeConfigurations).options " +
				"else (first f.nixosConfigurations).options.home-manager.users.type.getSubOptions [ ]"},
			"nix-darwin": map[string]any{"expr": flake + "(first f.darwinConfigurations).options"},
		},
	}
}
