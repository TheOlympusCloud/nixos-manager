package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// Stopping lets a request that is under way (a commit, a push) finish before
// serve returns, and the process exits.
func TestServeDrainsRequests(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started, finished := make(chan struct{}), false
	hs := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		finished = true
		io.WriteString(w, "committed")
	})}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan error)
	go func() { served <- serve(ctx, hs, ln) }()
	got := make(chan string)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			got <- err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		got <- string(b)
	}()
	<-started
	stop()
	if err := <-served; err != nil || !finished {
		t.Fatalf("serve returned %v with the request finished: %v", err, finished)
	}
	if body := <-got; body != "committed" {
		t.Fatalf("the request got %q", body)
	}
}
