package middleware

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/circuitbreaker"
    "net/http"
    "sync"
    "time"
)

var (
    breakers = make(map[string]*circuitbreaker.CircuitBreaker)
    mutex    sync.RWMutex
)

// CircuitBreakerMiddleware creates a circuit breaker for each endpoint
func CircuitBreakerMiddleware(name string) gin.HandlerFunc {
    mutex.Lock()
    if _, exists := breakers[name]; !exists {
        breakers[name] = circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{
            FailureThreshold: 5,
            ResetTimeout:    10 * time.Second,
            HalfOpenMaxCalls: 3,
        })
    }
    mutex.Unlock()

    return func(c *gin.Context) {
        mutex.RLock()
        breaker := breakers[name]
        mutex.RUnlock()

        err := breaker.Execute(func() error {
            c.Next()
            if c.Writer.Status() >= 500 {
                return http.ErrServerClosed
            }
            return nil
        })

        if err != nil {
            // Circuit is open
            c.AbortWithStatusJSON(503, gin.H{
                "error": "Service temporarily unavailable",
                "details": "Circuit breaker is open",
            })
            return
        }
    }
} 