package middleware

import (
	"net"
	"net/http"
	"strings"
)

// RealIPMiddleware extracts the original client IP from X-Forwarded-For or
// X-Real-IP headers when the direct connection comes from a trusted proxy.
// When no trusted proxies are configured, r.RemoteAddr is left unchanged.
type RealIPMiddleware struct {
	trustedNets []*net.IPNet
	trustedIPs  []net.IP
}

// NewRealIPMiddleware creates a RealIPMiddleware from a list of trusted CIDR
// ranges or bare IP addresses. Invalid entries are silently skipped.
func NewRealIPMiddleware(trusted []string) *RealIPMiddleware {
	m := &RealIPMiddleware{}
	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// Try CIDR first.
		if _, cidr, err := net.ParseCIDR(entry); err == nil {
			m.trustedNets = append(m.trustedNets, cidr)
			continue
		}
		// Fall back to bare IP.
		if ip := net.ParseIP(entry); ip != nil {
			m.trustedIPs = append(m.trustedIPs, ip)
		}
	}
	return m
}

// Enabled returns true when at least one trusted proxy is configured.
func (m *RealIPMiddleware) Enabled() bool {
	return len(m.trustedNets) > 0 || len(m.trustedIPs) > 0
}

// Wrap returns an http.Handler that rewrites r.RemoteAddr to the real client
// IP when the direct peer is a trusted proxy.
func (m *RealIPMiddleware) Wrap(next http.Handler) http.Handler {
	if !m.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peerIP := extractIP(r.RemoteAddr)
		if m.isTrusted(peerIP) {
			if realIP := resolveRealIP(r); realIP != "" {
				r.RemoteAddr = realIP
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isTrusted checks whether the given IP is in the trusted set.
func (m *RealIPMiddleware) isTrusted(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, cidr := range m.trustedNets {
		if cidr.Contains(ip) {
			return true
		}
	}
	for _, trusted := range m.trustedIPs {
		if trusted.Equal(ip) {
			return true
		}
	}
	return false
}

// resolveRealIP reads X-Forwarded-For (first entry) or X-Real-IP to determine
// the original client IP.
func resolveRealIP(r *http.Request) string {
	// Prefer X-Forwarded-For — take the left-most (client) entry.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		candidate := strings.TrimSpace(parts[0])
		if ip := net.ParseIP(candidate); ip != nil {
			return candidate
		}
	}
	// Fall back to X-Real-IP.
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		candidate := strings.TrimSpace(xri)
		if ip := net.ParseIP(candidate); ip != nil {
			return candidate
		}
	}
	return ""
}

// extractIP parses the IP portion from an address that may include a port
// (e.g. "192.168.1.1:12345" or "[::1]:12345").
func extractIP(addr string) net.IP {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// addr might be a bare IP without port.
		return net.ParseIP(addr)
	}
	return net.ParseIP(host)
}
