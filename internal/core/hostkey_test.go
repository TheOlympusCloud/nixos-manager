package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshServer serves SSH handshakes on 127.0.0.1 with these host keys, and
// counts every login attempt and every handshake that got that far.
func sshServer(t *testing.T, keys ...ssh.Signer) (addr string, logins *atomic.Int32) {
	t.Helper()
	logins = new(atomic.Int32)
	cfg := &ssh.ServerConfig{
		NoClientAuth:         true, // so a login without credentials is counted too
		NoClientAuthCallback: func(ssh.ConnMetadata) (*ssh.Permissions, error) { logins.Add(1); return &ssh.Permissions{}, nil },
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			logins.Add(1)
			return &ssh.Permissions{}, nil
		},
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			logins.Add(1)
			return &ssh.Permissions{}, nil
		},
		KeyboardInteractiveCallback: func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			logins.Add(1)
			return &ssh.Permissions{}, nil
		},
	}
	for _, k := range keys {
		cfg.AddHostKey(k)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if sc, chans, reqs, err := ssh.NewServerConn(c, cfg); err == nil {
					go ssh.DiscardRequests(reqs)
					for ch := range chans {
						ch.Reject(ssh.Prohibited, "no")
					}
					sc.Close()
				}
			}()
		}
	}()
	return l.Addr().String(), logins
}

func ed25519Signer(t *testing.T) ssh.Signer {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFetchHostKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, _ := ssh.NewSignerFromKey(rsaKey)
	host := ed25519Signer(t)
	// A machine offers several host keys: the ed25519 one is what sops-nix uses.
	addr, logins := sshServer(t, rsaSigner, host)

	got, err := FetchHostKey(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(host.PublicKey())))
	if got.Key != want || got.Fingerprint != ssh.FingerprintSHA256(host.PublicKey()) {
		t.Fatalf("got %+v, want %s", got, want)
	}
	// What Settings → Secrets accepts.
	if pub, err := parseHostKey(got.Key); err != nil || ssh.FingerprintSHA256(pub) != got.Fingerprint {
		t.Fatalf("not a key Secrets takes: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let the server see whatever the client sent
	if n := logins.Load(); n != 0 {
		t.Fatalf("the fetch tried to log in %d times: it must stop at the host key", n)
	}
}

func TestFetchHostKeyWithoutEd25519(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, _ := ssh.NewSignerFromKey(rsaKey)
	addr, logins := sshServer(t, rsaSigner)
	_, err = FetchHostKey(context.Background(), addr)
	isErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), "ed25519") {
		t.Fatalf("error %v", err)
	}
	if logins.Load() != 0 {
		t.Fatal("logged in")
	}
}

func TestFetchHostKeyUnreachable(t *testing.T) {
	// Nothing listens there.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	_, err := FetchHostKey(context.Background(), addr)
	isErr(t, err, ErrInvalid)

	// Something that never answers: the caller's deadline ends the wait.
	quiet, _ := net.Listen("tcp", "127.0.0.1:0")
	defer quiet.Close()
	go func() {
		for {
			c, err := quiet.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = FetchHostKey(ctx, quiet.Addr().String())
	isErr(t, err, ErrInvalid)
	if time.Since(start) > 5*time.Second {
		t.Fatalf("waited %v past the deadline", time.Since(start))
	}
}

func TestSSHAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.50":      "192.168.1.50:22",
		" nas.lan ":         "nas.lan:22",
		"nas":               "nas:22",
		"nas:2222":          "nas:2222",
		"fe80::1":           "[fe80::1]:22",
		"[fe80::1]:2222":    "[fe80::1]:2222",
		"10.0.0.1:22":       "10.0.0.1:22",
		"my-host.example.o": "my-host.example.o:22",
	} {
		if got, err := sshAddress(in); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "root@nas", "nas:0", "nas:99999", "nas:ssh", "-oProxyCommand=x", "nas;reboot", "http://nas", "nas/", "a..b"} {
		if _, err := sshAddress(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
