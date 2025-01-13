package middleware

import "github.com/gin-gonic/gin"

func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Prevent clickjacking
        c.Header("X-Frame-Options", "DENY")
        // XSS protection
        c.Header("X-XSS-Protection", "1; mode=block")
        // Prevent MIME type sniffing
        c.Header("X-Content-Type-Options", "nosniff")
        // HSTS
        c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        // CSP
        c.Header("Content-Security-Policy", "default-src 'self'")
        // Referrer Policy
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        
        c.Next()
    }
} 