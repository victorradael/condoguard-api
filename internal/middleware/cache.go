package middleware

import (
    "fmt"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/cache"
)

func CacheMiddleware(duration time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Skip caching for non-GET requests
        if c.Request.Method != "GET" {
            c.Next()
            return
        }

        // Create cache key from request URL
        cacheKey := fmt.Sprintf("cache:%s", c.Request.URL.String())
        
        // Try to get from cache
        redisCache := cache.NewRedisCache()
        var cachedResponse interface{}
        err := redisCache.Get(cacheKey, &cachedResponse)
        
        if err == nil {
            // Return cached response
            c.JSON(200, cachedResponse)
            c.Abort()
            return
        }

        // Store the original response writer
        writer := c.Writer
        responseData := make([]byte, 0)
        
        // Create a new response writer to capture the response
        c.Writer = &responseWriter{
            ResponseWriter: writer,
            responseData:  &responseData,
        }

        // Process request
        c.Next()

        // Cache the response if status is 200
        if c.Writer.Status() == 200 {
            redisCache.Set(cacheKey, string(responseData), duration)
        }
    }
}

type responseWriter struct {
    gin.ResponseWriter
    responseData *[]byte
}

func (w *responseWriter) Write(b []byte) (int, error) {
    *w.responseData = append(*w.responseData, b...)
    return w.ResponseWriter.Write(b)
} 