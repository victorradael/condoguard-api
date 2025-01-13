package monitoring

import (
    "context"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/victorradael/condoguard/internal/circuitbreaker"
    "github.com/victorradael/condoguard/internal/logger"
    "sync"
    "time"
)

var (
    circuitBreakerState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "circuit_breaker_state",
            Help: "Current state of circuit breakers (0: Closed, 1: Half-Open, 2: Open)",
        },
        []string{"name"},
    )

    circuitBreakerTrips = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "circuit_breaker_trips_total",
            Help: "Total number of times circuit breakers have tripped",
        },
        []string{"name"},
    )

    circuitBreakerFailures = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "circuit_breaker_failures_total",
            Help: "Total number of failures recorded by circuit breakers",
        },
        []string{"name"},
    )
)

type CircuitMonitor struct {
    breakers    map[string]*circuitbreaker.CircuitBreaker
    alerter     *AlertManager
    mutex       sync.RWMutex
    stopChan    chan struct{}
}

func NewCircuitMonitor(alertManager *AlertManager) *CircuitMonitor {
    return &CircuitMonitor{
        breakers: make(map[string]*circuitbreaker.CircuitBreaker),
        alerter:  alertManager,
        stopChan: make(chan struct{}),
    }
}

func (cm *CircuitMonitor) RegisterBreaker(name string, breaker *circuitbreaker.CircuitBreaker) {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    cm.breakers[name] = breaker
}

func (cm *CircuitMonitor) Start(ctx context.Context) {
    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()

    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case <-cm.stopChan:
                return
            case <-ticker.C:
                cm.checkCircuits()
            }
        }
    }()
}

func (cm *CircuitMonitor) Stop() {
    close(cm.stopChan)
}

func (cm *CircuitMonitor) checkCircuits() {
    cm.mutex.RLock()
    defer cm.mutex.RUnlock()

    for name, breaker := range cm.breakers {
        state := breaker.GetState()
        circuitBreakerState.WithLabelValues(name).Set(float64(state))

        // Alert on state changes
        if state == circuitbreaker.StateOpen {
            cm.alerter.SendAlert(Alert{
                Level:   AlertLevelWarning,
                Title:   "Circuit Breaker Opened",
                Message: "Circuit breaker for " + name + " has opened",
                Tags:    []string{"circuit-breaker", name},
            })

            logger.Warn("Circuit breaker opened", logger.Fields{
                "service": name,
                "state":   "open",
            })
        }
    }
} 