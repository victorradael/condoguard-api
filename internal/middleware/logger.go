package middleware

import (
    "github.com/gin-gonic/gin"
    "log"
    "time"
)

func Logger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        method := c.Request.Method

        // Process request
        c.Next()

        // Log details after request is processed
        latency := time.Since(start)
        statusCode := c.Writer.Status()
        log.Printf("[%s] %s %d %s", method, path, statusCode, latency)
    }
} 