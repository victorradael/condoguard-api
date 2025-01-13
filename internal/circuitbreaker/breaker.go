package circuitbreaker

import (
    "errors"
    "sync"
    "time"
    "github.com/victorradael/condoguard/internal/logger"
)

type State int

const (
    StateClosed State = iota    // Circuit is closed - requests are allowed
    StateOpen                   // Circuit is open - requests are not allowed
    StateHalfOpen              // Circuit is half-open - limited requests allowed
)

type CircuitBreaker struct {
    state           State
    failureCount    int
    lastFailureTime time.Time
    mutex           sync.RWMutex

    // Configuration
    failureThreshold   int
    resetTimeout       time.Duration
    halfOpenMaxCalls   int
    currentHalfOpen   int
}

type Config struct {
    FailureThreshold int
    ResetTimeout     time.Duration
    HalfOpenMaxCalls int
}

func NewCircuitBreaker(config Config) *CircuitBreaker {
    return &CircuitBreaker{
        state:            StateClosed,
        failureThreshold: config.FailureThreshold,
        resetTimeout:     config.ResetTimeout,
        halfOpenMaxCalls: config.HalfOpenMaxCalls,
    }
}

func (cb *CircuitBreaker) Execute(operation func() error) error {
    if !cb.allowRequest() {
        return errors.New("circuit breaker is open")
    }

    err := operation()
    cb.recordResult(err)
    return err
}

func (cb *CircuitBreaker) allowRequest() bool {
    cb.mutex.RLock()
    defer cb.mutex.RUnlock()

    switch cb.state {
    case StateClosed:
        return true
    case StateOpen:
        if time.Since(cb.lastFailureTime) > cb.resetTimeout {
            cb.mutex.RUnlock()
            cb.mutex.Lock()
            cb.state = StateHalfOpen
            cb.currentHalfOpen = 0
            cb.mutex.Unlock()
            cb.mutex.RLock()
            return true
        }
        return false
    case StateHalfOpen:
        return cb.currentHalfOpen < cb.halfOpenMaxCalls
    default:
        return false
    }
}

func (cb *CircuitBreaker) recordResult(err error) {
    cb.mutex.Lock()
    defer cb.mutex.Unlock()

    if err != nil {
        cb.failureCount++
        cb.lastFailureTime = time.Now()

        if cb.state == StateHalfOpen || cb.failureCount >= cb.failureThreshold {
            cb.state = StateOpen
            logger.Warn("Circuit breaker opened", logger.Fields{
                "failures": cb.failureCount,
                "lastError": err.Error(),
            })
        }
    } else {
        switch cb.state {
        case StateHalfOpen:
            cb.currentHalfOpen++
            if cb.currentHalfOpen >= cb.halfOpenMaxCalls {
                cb.state = StateClosed
                cb.failureCount = 0
                logger.Info("Circuit breaker closed", logger.Fields{
                    "successfulCalls": cb.currentHalfOpen,
                })
            }
        case StateClosed:
            cb.failureCount = 0
        }
    }
}

func (cb *CircuitBreaker) GetState() State {
    cb.mutex.RLock()
    defer cb.mutex.RUnlock()
    return cb.state
} 