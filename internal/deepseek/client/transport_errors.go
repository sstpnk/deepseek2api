package client

import (
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
)

// isTransportError reports whether err looks like a network/transport-layer
// failure (DNS, dial, EOF, SOCKS handshake, reset, refused connection, etc.)
// rather than an HTTP-level upstream error such as a 4xx/5xx response.
//
// Only transport errors should mark a proxy as unhealthy; upstream business
// errors (auth failure, content filter, etc.) must not.
func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	keywords := []string{
		"socks",
		"dial tcp",
		"dial udp",
		"i/o timeout",
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"tls handshake",
		"unexpected eof",
		"transport failed",
		"server misbehaving",
		"network is unreachable",
		"host is unreachable",
	}
	for _, kw := range keywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
