// Command egressproxy is the experiment's narrow coordinator egress relay.
// Every accepted TCP connection is forwarded to one fixed TLS authority; the
// client cannot select another destination.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:443", "TCP relay listen address")
	health := flag.String("health", "0.0.0.0:8080", "health HTTP listen address")
	upstream := flag.String("upstream", "openrouter.ai:443", "fixed upstream TLS authority")
	flag.Parse()
	if flag.NArg() != 0 || !validAuthority(*upstream) {
		fmt.Fprintln(os.Stderr, "egressproxy: -upstream must be one exact DNS host:port authority and positional arguments are forbidden")
		os.Exit(2)
	}

	healthServer := &http.Server{
		Addr:              *health,
		Handler:           http.HandlerFunc(healthHandler),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if err := healthServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("egress relay listening on %s; fixed upstream %s", *listen, strings.ToLower(*upstream))
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for {
		client, err := listener.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go relay(client, strings.ToLower(*upstream), dialer)
	}
}

func validAuthority(authority string) bool {
	host, port, err := net.SplitHostPort(authority)
	return err == nil && host != "" && port != "" && net.ParseIP(host) == nil && !strings.ContainsAny(host, "/@")
}

func healthHandler(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.URL.Path != "/healthz" {
		http.NotFound(response, request)
		return
	}
	response.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(response, "ok\n")
}

func relay(client net.Conn, upstreamAuthority string, dialer *net.Dialer) {
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	upstream, err := dialer.DialContext(ctx, "tcp", upstreamAuthority)
	cancel()
	if err != nil {
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	copyOne := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		if closer, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyOne(upstream, client)
	go copyOne(client, upstream)
	<-done
	_ = client.Close()
	_ = upstream.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
