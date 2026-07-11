package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownServerDrainsActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	requestResult := make(chan error, 1)
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_, requestErr = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestResult <- requestErr
	}()
	<-started
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- shutdownServer(server, time.Second) }()
	select {
	case err := <-shutdownResult:
		t.Fatalf("shutdown returned before active request drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-shutdownResult; err != nil {
		t.Fatal(err)
	}
	if err := <-requestResult; err != nil {
		t.Fatal(err)
	}
	if err := <-serveResult; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("unexpected serve result %v", err)
	}
}

func TestShutdownServerCancelsRequestAfterDeadline(t *testing.T) {
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(handlerDone)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(listener) }()
	go func() {
		response, requestErr := http.Get("http://" + listener.Addr().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
	}()
	<-started
	err = shutdownServer(server, 25*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected shutdown result %v", err)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler context was not canceled")
	}
}
