package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// ParseOrigins parses a comma-separated list of allowed browser origins
// (scheme://host[:port], as sent in the Origin header). The wildcard "*" is
// rejected: the API uses credentials (cookies), and a wildcard would let any
// site read authenticated responses.
func ParseOrigins(raw string) ([]string, error) {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			return nil, fmt.Errorf("origin %q: wildcard is not allowed with credentials; list the origins", o)
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || o != u.Scheme+"://"+u.Host {
			return nil, fmt.Errorf("origin %q: must be scheme://host[:port] with no path or trailing slash", o)
		}
		out = append(out, o)
	}
	return out, nil
}

// originAllowed reports whether the request's Origin header is in allowed.
func originAllowed(r *http.Request, allowed []string) bool {
	o := r.Header.Get("Origin")
	return o != "" && slices.Contains(allowed, o)
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// CheckCookieOrigin protects cookie-authenticated, state-changing requests
// against CSRF: they must come from one of the allowed origins. Requests that
// carry the credential in the Authorization header (the app) are not exposed
// to CSRF and are not checked.
func CheckCookieOrigin(r *http.Request, allowed []string) bool {
	return isSafeMethod(r.Method) || originAllowed(r, allowed)
}
