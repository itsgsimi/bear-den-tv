// Listeners: bind only the chosen interface, optional TLS, and serve
// (docs/security.md).

package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// refusedInterfacePrefixes names interface classes that are never a home LAN:
// containers, bridges, virtual pairs, and VPN tunnels.
var refusedInterfacePrefixes = []string{"docker", "br-", "veth", "tun", "wg", "virbr"}

// BindInterface listens on every address of exactly one network interface
// (IPv4 and IPv6, link-local IPv6 with its zone) on port. Loopback and
// container/bridge/VPN interfaces are refused by name and flag. With port 0
// each address receives its own ephemeral port. On any failure every listener
// opened so far is closed. The returned URLs use the http scheme; use
// ListenerURLs for https.
func BindInterface(ifaceName string, port int) ([]net.Listener, []string, error) {
	if ifaceName == "" {
		return nil, nil, errors.New("remote: interface name is required (remote.interfaces)")
	}
	if ifaceName == "lo" || refusedInterfaceName(ifaceName) {
		return nil, nil, fmt.Errorf("remote: interface %q is loopback, a container/bridge, or a VPN tunnel and cannot serve the LAN remote; choose the wired or Wi-Fi interface", ifaceName)
	}
	if port < 0 || port > 65535 {
		return nil, nil, fmt.Errorf("remote: port %d is out of range", port)
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("remote: interface %q not found: %w", ifaceName, err)
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return nil, nil, fmt.Errorf("remote: interface %q is a loopback interface and cannot serve the LAN remote", ifaceName)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, nil, fmt.Errorf("remote: reading addresses of %q: %w", ifaceName, err)
	}
	var listeners []net.Listener
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		host := ipn.IP.String()
		if ipn.IP.To4() == nil && ipn.IP.IsLinkLocalUnicast() {
			host += "%" + ifaceName
		}
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			closeListeners(listeners)
			return nil, nil, fmt.Errorf("remote: binding %s on %q: %w", net.JoinHostPort(host, strconv.Itoa(port)), ifaceName, err)
		}
		listeners = append(listeners, l)
	}
	if len(listeners) == 0 {
		return nil, nil, fmt.Errorf("remote: interface %q has no IP addresses to bind", ifaceName)
	}
	return listeners, ListenerURLs(listeners, false), nil
}

func refusedInterfaceName(name string) bool {
	for _, p := range refusedInterfacePrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// BindDev listens on a loopback literal such as "127.0.0.1:0" or "[::1]:0"
// for --dev-listen; any other host is refused.
func BindDev(addr string) ([]net.Listener, []string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, nil, fmt.Errorf("remote: --dev-listen must be host:port such as 127.0.0.1:0: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, nil, fmt.Errorf("remote: --dev-listen accepts loopback literals only (127.0.0.1:0 or [::1]:0), got %q", host)
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, nil, fmt.Errorf("remote: binding dev listener %s: %w", addr, err)
	}
	ls := []net.Listener{l}
	return ls, ListenerURLs(ls, false), nil
}

func closeListeners(ls []net.Listener) {
	for _, l := range ls {
		_ = l.Close()
	}
}

// ListenerHosts returns the literal host:port of each listener, the values
// Options.AllowedHosts needs (IPv6 bracketed, zone kept).
func ListenerHosts(ls []net.Listener) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Addr().String())
	}
	return out
}

// ListenerURLs returns "scheme://host:port/" for each listener with the IPv6
// zone percent-encoded as %25.
func ListenerURLs(ls []net.Listener, https bool) []string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	out := make([]string, 0, len(ls))
	for _, h := range ListenerHosts(ls) {
		out = append(out, scheme+"://"+strings.ReplaceAll(h, "%", "%25")+"/")
	}
	return out
}

// LoadTLS reads a PEM certificate chain and private key for the HTTPS
// transport, checking the leaf's validity window; NextProtos is pinned to
// HTTP/1.1 because the WebSocket endpoint does not run over HTTP/2.
func LoadTLS(certFile, keyFile string) (*tls.Config, error) {
	if _, err := os.Stat(certFile); err != nil {
		return nil, fmt.Errorf("remote: TLS certificate file %q: %w (set remote.tls.cert_file to a PEM certificate chain)", certFile, err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return nil, fmt.Errorf("remote: TLS key file %q: %w (set remote.tls.key_file to the matching PEM private key)", keyFile, err)
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("remote: loading TLS key pair %q + %q: %w (both must be PEM and the key must match the certificate)", certFile, keyFile, err)
	}
	leaf := cert.Leaf
	if leaf == nil && len(cert.Certificate) > 0 {
		leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("remote: parsing TLS certificate %q: %w", certFile, err)
		}
	}
	if leaf != nil {
		now := time.Now()
		if now.After(leaf.NotAfter) {
			return nil, fmt.Errorf("remote: TLS certificate %q expired on %s; renew it", certFile, leaf.NotAfter.Format(time.RFC3339))
		}
		if now.Before(leaf.NotBefore) {
			return nil, fmt.Errorf("remote: TLS certificate %q is not valid before %s", certFile, leaf.NotBefore.Format(time.RFC3339))
		}
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"},
	}, nil
}

// ListenAndServe serves on every listener (wrapped in TLS when tlsConfig is
// non-nil) until ctx is done or a listener fails, then shuts down gracefully
// within Options.ShutdownTimeout, closes every WebSocket with 1001, and
// releases the server. It returns nil after a clean ctx-driven stop.
func (s *Server) ListenAndServe(ctx context.Context, listeners []net.Listener, tlsConfig *tls.Config) error {
	if len(listeners) == 0 {
		return errors.New("remote: ListenAndServe needs at least one listener")
	}
	srv := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, len(listeners))
	for _, l := range listeners {
		if tlsConfig != nil {
			l = tls.NewListener(l, tlsConfig)
		}
		go func(l net.Listener) { errc <- srv.Serve(l) }(l)
	}
	var firstErr error
	pending := len(listeners)
	select {
	case <-ctx.Done():
	case err := <-errc:
		pending--
		if !errors.Is(err, http.ErrServerClosed) {
			firstErr = err
		}
	}
	sctx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}
	for ; pending > 0; pending-- {
		if err := <-errc; firstErr == nil && !errors.Is(err, http.ErrServerClosed) {
			firstErr = err
		}
	}
	_ = s.Close()
	return firstErr
}
