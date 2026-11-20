package core

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostKey is a machine's SSH ed25519 host key, as FetchHostKey read it.
type HostKey struct {
	Key         string `json:"key"`         // ssh-ed25519 AAAA…
	Fingerprint string `json:"fingerprint"` // SHA256:…, as ssh-keygen -l prints it
}

var dnsName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// sshAddress is host:port of a host name or IP address, with an optional port (22 by default).
func sshAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	host, port := address, "22"
	if net.ParseIP(address) == nil {
		if h, p, err := net.SplitHostPort(address); err == nil {
			host, port = h, p
		}
	}
	n, err := strconv.Atoi(port)
	if (net.ParseIP(host) == nil && !dnsName.MatchString(host)) || len(host) > 253 || err != nil || n < 1 || n > 65535 {
		return "", errorf(ErrInvalid, "an address is a host name or IP address, optionally with :port")
	}
	return net.JoinHostPort(host, port), nil
}

var errGotKey = errors.New("got the host key")

// FetchHostKey asks the SSH server at address for its ed25519 host key. It
// only starts the handshake: the server proves it holds the key before any
// login, and the connection ends there, so no credential is sent and nothing
// runs on the machine. Anyone between here and the machine can answer with
// a key of their own: compare the fingerprint with the machine's.
func FetchHostKey(ctx context.Context, address string) (HostKey, error) {
	addr, err := sshAddress(address)
	if err != nil {
		return HostKey{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return HostKey{}, errorf(ErrInvalid, "cannot reach %s: %v", addr, err)
	}
	defer conn.Close()
	defer context.AfterFunc(ctx, func() { conn.Close() })()
	var key ssh.PublicKey
	_, _, _, err = ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User:              "root",
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			key = k
			return errGotKey // ends the handshake before authentication
		},
	})
	switch {
	case key != nil && key.Type() == ssh.KeyAlgoED25519:
		return HostKey{Key: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), Fingerprint: ssh.FingerprintSHA256(key)}, nil
	case ctx.Err() != nil:
		return HostKey{}, errorf(ErrInvalid, "%s did not answer as an SSH server in time", addr)
	case err != nil && strings.Contains(err.Error(), "no common algorithm for host key"):
		return HostKey{}, errorf(ErrInvalid, "%s offers no ed25519 host key, which sops-nix decrypts with", addr)
	default:
		return HostKey{}, errorf(ErrInvalid, "no SSH host key from %s: %v", addr, err)
	}
}
