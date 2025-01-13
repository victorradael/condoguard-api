package security

import (
    "crypto/tls"
    "net/http"
    "time"
)

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
    // TLS Configuration
    TLSConfig *tls.Config

    // HTTP Security Headers
    SecurityHeaders map[string]string

    // CORS Configuration
    CORSConfig CORSConfig

    // Rate Limiting
    RateLimitRequests int
    RateLimitDuration time.Duration
}

type CORSConfig struct {
    AllowedOrigins []string
    AllowedMethods []string
    AllowedHeaders []string
    MaxAge         time.Duration
}

// NewDefaultSecurityConfig returns a SecurityConfig with secure defaults
func NewDefaultSecurityConfig() *SecurityConfig {
    return &SecurityConfig{
        TLSConfig: &tls.Config{
            MinVersion:               tls.VersionTLS12,
            CurvePreferences:         []tls.CurveID{tls.X25519, tls.CurveP256},
            PreferServerCipherSuites: true,
            CipherSuites: []uint16{
                tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
                tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            },
        },
        SecurityHeaders: map[string]string{
            "X-Frame-Options":           "DENY",
            "X-Content-Type-Options":    "nosniff",
            "X-XSS-Protection":          "1; mode=block",
            "Strict-Transport-Security": "max-age=31536000; includeSubDomains",
            "Content-Security-Policy":   "default-src 'self'; frame-ancestors 'none'",
            "Referrer-Policy":          "strict-origin-when-cross-origin",
        },
        RateLimitRequests: 100,
        RateLimitDuration: time.Minute,
    }
} 