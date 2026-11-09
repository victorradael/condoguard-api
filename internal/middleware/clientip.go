package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

// ClientIP resolves the client IP once per request for the per-IP limiters.
// When the connection comes from a trusted proxy (RemoteAddr inside one of
// the trusted prefixes) and X-Real-IP holds exactly one valid IP, that IP is
// the client; otherwise the client is RemoteAddr and X-Real-IP is ignored.
// With no trusted prefixes the header is never read.
//
// The proxy must set X-Real-IP itself, replacing whatever the client sent
// (Caddy's header_up does). X-Forwarded-For is never read.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := remoteIP(r)
			if addr, err := netip.ParseAddr(ip); err == nil && contains(trusted, addr) {
				if real, ok := parseRealIP(r.Header.Values("X-Real-IP")); ok {
					ip = real
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, ip)))
		})
	}
}

// ParseTrustedProxies parses a comma-separated list of CIDRs (a bare IP is a
// single-address prefix). An empty list trusts no proxy.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		p, err := netip.ParsePrefix(f)
		if err != nil {
			a, aerr := netip.ParseAddr(f)
			if aerr != nil || a.Zone() != "" {
				return nil, fmt.Errorf("%q is not a CIDR or IP", f)
			}
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// clientIP is the IP the per-IP limiters key on: the one resolved by
// ClientIP, or RemoteAddr when that middleware is not installed.
func clientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return remoteIP(r)
}

// remoteIP is RemoteAddr without the port, IPv4-mapped IPv6 unmapped so a
// client has one key whatever the listener's address family.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a.Unmap().WithZone("").String()
	}
	return host
}

func parseRealIP(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	a, err := netip.ParseAddr(strings.TrimSpace(values[0]))
	if err != nil || a.Zone() != "" {
		return "", false
	}
	return a.Unmap().String(), true
}

func contains(prefixes []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap().WithZone("")
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
