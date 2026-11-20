package core

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Notifications: when a deployment fails or is stuck, a machine goes silent,
// the branch comin deploys does not evaluate, a release or input reaches its
// end of life, or a scheduled update of the inputs is ready to merge or
// failed, nixos-manager tells ntfy, a webhook or a mailbox, set in
// Settings (notify.json). Each event is sent once per channel; what was sent
// is kept in notify-state.json, so a restart does not send it again.

const (
	notifyEvery  = time.Minute
	notifyWarmup = 20 * time.Minute // after a start the machines report in again first (see silenceWarmup)
	notifyFirst  = time.Minute      // backoff after a failed send, doubling up to notifyLast
	notifyLast   = time.Hour
	sendTimeout  = 20 * time.Second
)

// NotifySettings are the channels, as saved and as Settings edits them.
// Secrets are write-only: read back empty, with Has* set; saved with Has*
// set and the secret empty, the saved one is kept.
type NotifySettings struct {
	ConsoleURL string          `json:"consoleUrl,omitempty"` // for links in messages, e.g. http://nixos-manager.home.lan:8080
	Ntfy       *NtfyChannel    `json:"ntfy,omitempty"`
	Webhook    *WebhookChannel `json:"webhook,omitempty"`
	Email      *EmailChannel   `json:"email,omitempty"`
}

// NtfyChannel posts to an ntfy topic: https://ntfy.sh/<topic>, or a server of your own.
type NtfyChannel struct {
	URL      string    `json:"url"`
	Token    string    `json:"token,omitempty"` // an access token, for a protected topic
	HasToken bool      `json:"hasToken,omitempty"`
	Since    time.Time `json:"since"` // events first seen before this are not sent here
}

// WebhookChannel posts each event as JSON.
type WebhookChannel struct {
	URL   string    `json:"url"`
	Since time.Time `json:"since"`
}

// EmailChannel sends mail over SMTP.
type EmailChannel struct {
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Security    string    `json:"security"` // starttls, tls (implicit, port 465) or none
	Username    string    `json:"username,omitempty"`
	Password    string    `json:"password,omitempty"`
	HasPassword bool      `json:"hasPassword,omitempty"`
	From        string    `json:"from"`
	To          []string  `json:"to"`
	Since       time.Time `json:"since"`
}

// Event is something to tell: Key says which, so it is sent once per channel
// while it lasts.
type Event struct {
	Key      string   `json:"key"`
	Kind     string   `json:"kind"`     // deploy-failed, deploy-stuck, host-silent, eval-failed, eol, update-ready, update-failed or test
	Severity string   `json:"severity"` // error, warning, info
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Hosts    []string `json:"hosts"`
	Link     string   `json:"link,omitempty"` // a console route, e.g. #/deployments
}

// ChannelStatus is how the last send to a channel went, for Settings.
type ChannelStatus struct {
	Channel string    `json:"channel"`
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	At      time.Time `json:"at"`
}

type sentEvent struct {
	First time.Time `json:"first"` // when it was first seen
	Sent  []string  `json:"sent"`  // the channels that have it
}

type backoff struct {
	next  time.Time
	delay time.Duration
}

// Notifier watches the fleet and sends what changed. Tick runs once a
// minute from Run.
type Notifier struct {
	Svc     *Service
	Client  *http.Client   // ntfy and the webhook; nil uses a client with a timeout
	RootCAs *x509.CertPool // SMTP over TLS; nil uses the system's
	// events lists what is true now, and which sources ("deploy", "eval", "update") it
	// could look at: an event of a source it could not look at is not over.
	events func(ctx context.Context, now time.Time) ([]Event, map[string]bool)

	path, statePath string
	started         time.Time

	mu       sync.Mutex
	settings NotifySettings
	state    map[string]sentEvent
	status   map[string]ChannelStatus
	waits    map[string]backoff
	retried  string // the fleet branch tree evaluated again once before its failure is told
}

// OpenNotifier loads dataDir/notify.json and notify-state.json.
func OpenNotifier(dataDir string, svc *Service) (*Notifier, error) {
	n := &Notifier{Svc: svc, path: filepath.Join(dataDir, "notify.json"), statePath: filepath.Join(dataDir, "notify-state.json"), started: time.Now(),
		state: map[string]sentEvent{}, status: map[string]ChannelStatus{}, waits: map[string]backoff{}}
	n.events = n.fleetEvents
	for path, into := range map[string]any{n.path: &n.settings, n.statePath: &n.state} {
		b, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			if err := json.Unmarshal(b, into); err != nil {
				if path == n.statePath { // only means telling again what is still true
					slog.Warn("cannot read what was notified; current events may be sent again", "file", path, "err", err)
					n.state = map[string]sentEvent{}
					continue
				}
				return nil, errorf(ErrInvalid, "reading %s: %v", path, err)
			}
		}
	}
	return n, nil
}

// Settings returns the channels with their secrets left out, and how the last
// send to each went.
func (n *Notifier) Settings() (NotifySettings, []ChannelStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := redactNotify(n.settings)
	var st []ChannelStatus
	for _, c := range slices.Sorted(maps.Keys(n.status)) {
		st = append(st, n.status[c])
	}
	return s, st
}

func redactNotify(s NotifySettings) NotifySettings {
	s = s.clone()
	if c := s.Ntfy; c != nil {
		c.HasToken, c.Token = c.Token != "", ""
	}
	if c := s.Email; c != nil {
		c.HasPassword, c.Password = c.Password != "", ""
	}
	return s
}

func (s NotifySettings) clone() NotifySettings {
	if s.Ntfy != nil {
		c := *s.Ntfy
		s.Ntfy = &c
	}
	if s.Webhook != nil {
		c := *s.Webhook
		s.Webhook = &c
	}
	if s.Email != nil {
		c := *s.Email
		c.To = slices.Clone(c.To)
		s.Email = &c
	}
	return s
}

// Save validates and stores the channels. A channel that is new or sends
// somewhere else only gets events that start from now on.
func (n *Notifier) Save(in NotifySettings, now time.Time) (NotifySettings, error) {
	in = in.clone() // the caller's stay as they are, saved or refused
	if err := validNotify(&in); err != nil {
		return NotifySettings{}, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	old := n.settings
	if c := in.Ntfy; c != nil {
		c.Since = now
		if o := old.Ntfy; o != nil {
			if c.Token == "" && c.HasToken && sameHost(o.URL, c.URL) { // never hand a saved token to another server
				c.Token = o.Token
			}
			if o.URL == c.URL {
				c.Since = o.Since
			}
		}
		c.HasToken = false
	}
	if c := in.Webhook; c != nil {
		c.Since = now
		if o := old.Webhook; o != nil && o.URL == c.URL {
			c.Since = o.Since
		}
	}
	if c := in.Email; c != nil {
		c.Since = now
		if o := old.Email; o != nil {
			if c.Password == "" && c.HasPassword && o.Username == c.Username && o.Host == c.Host {
				c.Password = o.Password
			}
			if slices.Equal(o.To, c.To) {
				c.Since = o.Since
			}
		}
		c.HasPassword = false
		if c.Username != "" && c.Password == "" {
			return NotifySettings{}, errorf(ErrInvalid, "email: a user name needs its password")
		}
	}
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return NotifySettings{}, err
	}
	if err := writeAtomic(n.path, b); err != nil {
		return NotifySettings{}, err
	}
	n.settings = in
	for _, c := range []string{"ntfy", "webhook", "email"} {
		if !slices.Contains(channelsOf(in), c) {
			delete(n.status, c)
			delete(n.waits, c)
		}
	}
	return redactNotify(in), nil
}

func validNotify(s *NotifySettings) error {
	web := func(what, raw string, needPath bool) error {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (needPath && strings.Trim(u.Path, "/") == "") {
			if needPath {
				return errorf(ErrInvalid, "%s: want an http(s) address with the topic, like https://ntfy.sh/my-fleet", what)
			}
			return errorf(ErrInvalid, "%s: want an http(s) address", what)
		}
		return nil
	}
	s.ConsoleURL = strings.TrimRight(strings.TrimSpace(s.ConsoleURL), "/")
	if s.ConsoleURL != "" {
		if err := web("console address", s.ConsoleURL, false); err != nil {
			return err
		}
	}
	if c := s.Ntfy; c != nil {
		c.URL, c.Token = strings.TrimSpace(c.URL), strings.TrimSpace(c.Token)
		if err := web("ntfy", c.URL, true); err != nil {
			return err
		}
		if strings.ContainsAny(c.Token, "\r\n ") {
			return errorf(ErrInvalid, "ntfy: the token cannot contain spaces or line breaks")
		}
	}
	if c := s.Webhook; c != nil {
		c.URL = strings.TrimSpace(c.URL)
		if err := web("webhook", c.URL, false); err != nil {
			return err
		}
	}
	if c := s.Email; c != nil {
		c.Host, c.Username = strings.TrimSpace(c.Host), strings.TrimSpace(c.Username)
		switch {
		case c.Host == "" || strings.ContainsAny(c.Host, " /:\r\n"):
			return errorf(ErrInvalid, "email: want the SMTP server's host name")
		case c.Port < 1 || c.Port > 65535:
			return errorf(ErrInvalid, "email: the port must be 1 to 65535")
		case !slices.Contains([]string{"starttls", "tls", "none"}, c.Security):
			return errorf(ErrInvalid, "email: security must be starttls, tls or none")
		case c.Username != "" && c.Security == "none" && !localHost(c.Host):
			return errorf(ErrInvalid, "email: a password is only sent encrypted; use starttls or tls")
		case strings.ContainsAny(c.Username+c.Password, "\r\n"):
			return errorf(ErrInvalid, "email: the user name and password cannot contain line breaks")
		}
		from, err := mail.ParseAddress(c.From)
		if err != nil {
			return errorf(ErrInvalid, "email: the sender %q is not an address", c.From)
		}
		c.From = from.String()
		var to []string
		for _, a := range c.To {
			if a = strings.TrimSpace(a); a == "" {
				continue
			}
			addr, err := mail.ParseAddress(a)
			if err != nil {
				return errorf(ErrInvalid, "email: %q is not an address", a)
			}
			to = append(to, addr.Address)
		}
		if len(to) == 0 || len(to) > 20 {
			return errorf(ErrInvalid, "email: want 1 to 20 recipients")
		}
		c.To = to
	}
	return nil
}

func sameHost(a, b string) bool {
	x, err1 := url.Parse(a)
	y, err2 := url.Parse(b)
	return err1 == nil && err2 == nil && x.Scheme == y.Scheme && strings.EqualFold(x.Host, y.Host)
}

func localHost(h string) bool { return h == "localhost" || h == "127.0.0.1" || h == "::1" }

func channelsOf(s NotifySettings) []string {
	var out []string
	if s.Ntfy != nil {
		out = append(out, "ntfy")
	}
	if s.Webhook != nil {
		out = append(out, "webhook")
	}
	if s.Email != nil {
		out = append(out, "email")
	}
	return out
}

// Test sends a test message to every channel and says how each went.
func (n *Notifier) Test(ctx context.Context, now time.Time) ([]ChannelStatus, error) {
	n.mu.Lock()
	s := n.settings
	n.mu.Unlock()
	chans := channelsOf(s)
	if len(chans) == 0 {
		return nil, errorf(ErrInvalid, "no channel is set up: save one first")
	}
	ev := Event{Key: "test", Kind: "test", Severity: "info", Hosts: []string{}, Link: "#/settings",
		Title: "Test notification from nixos-manager", Message: "Notifications from nixos-manager reach you here."}
	var out []ChannelStatus
	for _, c := range chans {
		err := n.send(ctx, s, c, ev, now)
		out = append(out, n.record(c, err, now))
	}
	return out, nil
}

// record keeps how a send went, and backs a failing channel off; n.mu must not be held.
func (n *Notifier) record(channel string, err error, now time.Time) ChannelStatus {
	st := ChannelStatus{Channel: channel, OK: err == nil, At: now}
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		st.Error = err.Error()
		w := n.waits[channel]
		w.delay = min(max(w.delay*2, notifyFirst), notifyLast)
		w.next = now.Add(w.delay)
		n.waits[channel] = w
	} else {
		delete(n.waits, channel)
	}
	n.status[channel] = st
	return st
}

// Run sends what changed every minute until ctx ends.
func (n *Notifier) Run(ctx context.Context) {
	t := time.NewTicker(notifyEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			n.Tick(ctx, now)
		}
	}
}

// Tick sends each event that is true now to the channels that do not have it
// yet, and forgets the events that are over, so they are sent again if they
// come back.
func (n *Notifier) Tick(ctx context.Context, now time.Time) {
	if now.Sub(n.started) < notifyWarmup {
		return
	}
	events, looked := n.events(ctx, now)
	n.mu.Lock()
	s, changed := n.settings, false
	active := map[string]bool{}
	for _, e := range events {
		active[e.Key] = true
		if _, ok := n.state[e.Key]; !ok {
			n.state[e.Key], changed = sentEvent{First: now, Sent: []string{}}, true
		}
	}
	for k := range n.state {
		if !active[k] && looked[sourceOf(k)] {
			delete(n.state, k)
			changed = true
		}
	}
	n.mu.Unlock()

	for _, e := range events {
		for _, c := range channelsOf(s) {
			n.mu.Lock()
			st, w := n.state[e.Key], n.waits[c]
			due := !slices.Contains(st.Sent, c) && !st.First.Before(since(s, c)) && !now.Before(w.next)
			n.mu.Unlock()
			if !due {
				continue
			}
			err := n.send(ctx, s, c, e, now)
			n.record(c, err, now)
			if err != nil {
				slog.Warn("cannot send a notification", "channel", c, "event", e.Key, "err", err)
				continue
			}
			n.mu.Lock()
			st = n.state[e.Key]
			st.Sent = append(st.Sent, c)
			n.state[e.Key], changed = st, true
			n.mu.Unlock()
		}
	}
	if changed {
		n.mu.Lock()
		b, err := json.MarshalIndent(n.state, "", "  ")
		n.mu.Unlock()
		if err == nil {
			err = writeAtomic(n.statePath, b)
		}
		if err != nil {
			slog.Warn("cannot save what was notified; after a restart it may be sent again", "err", err)
		}
	}
}

// sourceOf says which source an event key comes from.
func sourceOf(key string) string {
	if strings.HasPrefix(key, "eval-failed") || strings.HasPrefix(key, "eol:") {
		return "eval"
	}
	if strings.HasPrefix(key, "update-") {
		return "update"
	}
	return "deploy"
}

func since(s NotifySettings, channel string) time.Time {
	switch channel {
	case "ntfy":
		return s.Ntfy.Since
	case "webhook":
		return s.Webhook.Since
	default:
		return s.Email.Since
	}
}

// ---- what to tell -------------------------------------------------------------

// fleetEvents looks at the latest update of the inputs, at the deployments,
// as Health does (with the working copy's machines), and at the commit the
// fleet's branch points at, which is what the machines deploy: whether it
// evaluates, and which of its releases and inputs ended.
func (n *Notifier) fleetEvents(ctx context.Context, now time.Time) ([]Event, map[string]bool) {
	looked := map[string]bool{}
	var out []Event
	if u := n.Svc.Updates; u != nil {
		looked["update"] = true
		out = u.events(ctx)
	}
	working, _, err := n.Svc.Jobs.Model(ctx, "working")
	if err != nil || working == nil || working.Error != "" || n.Svc.Deploys == nil {
		return out, looked // without the fleet's machines nothing else is certain
	}
	ov, err := n.Svc.Deploys.Overview(ctx, working, now)
	if err != nil {
		return out, looked
	}
	looked["deploy"] = true
	out = append(out, deployEvents(working, ov, now)...)
	if len(ov.Commits) == 0 {
		return out, looked
	}
	tip := ov.Commits[0]
	m, _, err := n.Svc.Jobs.Model(ctx, "rev:"+tip.SHA)
	if err != nil || m == nil {
		return out, looked // still evaluating
	}
	if modelFailed(m) && n.retried != m.Tree {
		// Tell only a failure that happens twice: the first may be the network.
		n.retried = m.Tree
		if err := n.Svc.Jobs.prepareModel(m.Tree); err != nil {
			slog.Warn("cannot evaluate the fleet's branch again", "err", err)
		}
		return out, looked
	}
	looked["eval"] = true
	out = append(out, evalEvents(ov.Branch, tip, m)...)
	for _, in := range Insights(m, nil, "", nil, now) {
		if in.ID == "release-eol" || in.ID == "release-eol-soon" || strings.HasPrefix(in.ID, "input-eol:") {
			out = append(out, Event{Key: "eol:" + in.ID + ":" + in.Title, Kind: "eol", Severity: in.Severity, Title: in.Title,
				Message: in.Detail, Hosts: in.Hosts, Link: "#/health"})
		}
	}
	return out, looked
}

// deployEvents are the failed, stuck and silent machines of the fleet.
func deployEvents(m *Model, ov DeployOverview, now time.Time) []Event {
	var out []Event
	for _, d := range ov.Machines {
		if !slices.ContainsFunc(m.Hosts, func(h Host) bool { return h.Name == d.Host }) && !slices.Contains(m.Darwin, d.Host) {
			continue // not in this fleet (any more)
		}
		hosts := []string{d.Host}
		if f := d.Failure; d.State == "failed" && f != nil {
			what := map[bool]string{true: "build", false: "deploy"}[f.Phase == "build"]
			msg, _, _ := strings.Cut(strings.TrimSpace(f.Error), "\n")
			msg = cmpOr(truncate(msg, 300), "comin gave no error message.")
			if d.Commit != "" {
				msg += fmt.Sprintf(" It still runs %.7s.", d.Commit)
			}
			out = append(out, Event{Key: "deploy-failed:" + d.Host + ":" + f.Commit, Kind: "deploy-failed", Severity: "error", Hosts: hosts, Link: "#/deployments",
				Title: fmt.Sprintf("%s failed to %s %.7s", d.Host, what, f.Commit), Message: msg})
		}
		if d.State == "behind" && !d.Silent && now.Sub(d.BehindSince) > behindAfter {
			out = append(out, Event{Key: "deploy-stuck:" + d.Host, Kind: "deploy-stuck", Severity: "warning", Hosts: hosts, Link: "#/deployments",
				Title: fmt.Sprintf("%s has not caught up for over an hour", d.Host),
				Message: fmt.Sprintf("It runs %.7s, %d commit%s behind %s. comin may fail to evaluate or build the new one, or cannot reach Forgejo: journalctl -u comin on %s says why.",
					d.Commit, d.Behind, map[bool]string{true: "", false: "s"}[d.Behind == 1], d.Branch, d.Host)})
		}
		if d.Silent {
			out = append(out, Event{Key: "host-silent:" + d.Host, Kind: "host-silent", Severity: "warning", Hosts: hosts, Link: "#/deployments",
				Title: fmt.Sprintf("%s stopped reporting", d.Host),
				Message: fmt.Sprintf("No report or heartbeat since %s, more than the %d hours it may stay silent. It may be off, comin or its report timer may have stopped, or it cannot reach this console.",
					d.LastSeen.UTC().Format("2 Jan 15:04 MST"), d.SilentAfter)})
		}
	}
	return out
}

// evalEvents say that the branch the machines deploy does not evaluate, as a
// whole or for some machines.
func evalEvents(branch string, tip Commit, m *Model) []Event {
	var out []Event
	first := func(err string) string {
		lines := strings.Split(strings.TrimSpace(err), "\n")
		return truncate(strings.Join(lines[:min(len(lines), 4)], "\n"), 600)
	}
	if m.Error != "" {
		return []Event{{Key: "eval-failed", Kind: "eval-failed", Severity: "error", Hosts: []string{}, Link: "#/health",
			Title:   fmt.Sprintf("%s does not evaluate", branch),
			Message: fmt.Sprintf("The flake at %.7s (%s) fails before any machine: comin cannot deploy it anywhere.\n%s", tip.SHA, tip.Subject, first(m.Error))}}
	}
	for _, h := range m.Hosts {
		if h.Error != "" {
			out = append(out, Event{Key: "eval-failed:" + h.Name, Kind: "eval-failed", Severity: "error", Hosts: []string{h.Name}, Link: "#/health",
				Title:   fmt.Sprintf("%s does not evaluate on %s", h.Name, branch),
				Message: fmt.Sprintf("At %.7s (%s) comin cannot deploy %s.\n%s", tip.SHA, tip.Subject, h.Name, first(h.Error))})
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---- sending ------------------------------------------------------------------

func (n *Notifier) send(ctx context.Context, s NotifySettings, channel string, e Event, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	link := ""
	if s.ConsoleURL != "" && e.Link != "" {
		link = s.ConsoleURL + "/" + e.Link
	}
	switch channel {
	case "ntfy":
		return n.ntfy(ctx, s.Ntfy, e, link)
	case "webhook":
		return n.webhook(ctx, s.Webhook, e, link, now)
	default:
		return n.email(ctx, s.Email, e, link, now)
	}
}

func (n *Notifier) client() *http.Client {
	if n.Client != nil {
		return n.Client
	}
	return &http.Client{Timeout: sendTimeout}
}

func (n *Notifier) post(req *http.Request) error {
	resp, err := n.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b := make([]byte, 300)
		k, _ := resp.Body.Read(b)
		return fmt.Errorf("%s answered %s: %s", req.URL.Host, resp.Status, strings.TrimSpace(string(b[:k])))
	}
	return nil
}

// ntfy publishes with ntfy's headers; the title is RFC 2047 encoded, which
// ntfy decodes.
func (n *Notifier) ntfy(ctx context.Context, c *NtfyChannel, e Event, link string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, strings.NewReader(e.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", mime.QEncoding.Encode("utf-8", e.Title))
	req.Header.Set("Priority", map[string]string{"error": "high", "warning": "default", "info": "low"}[e.Severity])
	req.Header.Set("Tags", map[string]string{"error": "rotating_light", "warning": "warning", "info": "information_source"}[e.Severity])
	if link != "" {
		req.Header.Set("Click", link)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return n.post(req)
}

// webhook posts the event as JSON, with the link and the time.
func (n *Notifier) webhook(ctx context.Context, c *WebhookChannel, e Event, link string, now time.Time) error {
	body, err := json.Marshal(struct {
		Event
		URL string    `json:"url,omitempty"`
		At  time.Time `json:"at"`
	}{e, link, now.UTC()})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nixos-manager")
	return n.post(req)
}

// email sends one plain text message over SMTP.
func (n *Notifier) email(ctx context.Context, c *EmailChannel, e Event, link string, now time.Time) error {
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	tlsConf := &tls.Config{ServerName: c.Host, RootCAs: n.RootCAs, MinVersion: tls.VersionTLS12}
	d := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if c.Security == "tls" {
		conn, err = (&tls.Dialer{NetDialer: d, Config: tlsConf}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()
	if err := cl.Hello("nixos-manager"); err != nil {
		return err
	}
	if c.Security == "starttls" {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%s does not offer STARTTLS", c.Host)
		}
		if err := cl.StartTLS(tlsConf); err != nil {
			return err
		}
	}
	if c.Username != "" {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	}
	from, _ := mail.ParseAddress(c.From) // validated when saved
	if err := cl.Mail(from.Address); err != nil {
		return err
	}
	for _, to := range c.To {
		if err := cl.Rcpt(to); err != nil {
			return err
		}
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d.%s@nixos-manager>\r\n",
		from.String(), strings.Join(c.To, ", "), mime.QEncoding.Encode("utf-8", "[nixos-manager] "+e.Title), now.Format(time.RFC1123Z), now.UnixNano(), sanitizeID(e.Key))
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&msg)
	body := e.Message
	if link != "" {
		body += "\n\n" + link
	}
	qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
	qp.Close()
	if _, err := w.Write(msg.Bytes()); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

// sanitizeID keeps the characters a Message-ID may hold.
func sanitizeID(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return r
		}
		return '-'
	}, s)
}
