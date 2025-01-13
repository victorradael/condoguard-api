package health

import (
    "context"
    "sync"
    "time"
    "github.com/victorradael/condoguard/internal/logger"
)

type Status string

const (
    StatusUp      Status = "UP"
    StatusDown    Status = "DOWN"
    StatusDegraded Status = "DEGRADED"
)

type Check struct {
    Name     string                 `json:"name"`
    Status   Status                 `json:"status"`
    Details  map[string]interface{} `json:"details,omitempty"`
    Error    string                 `json:"error,omitempty"`
    LastCheck time.Time            `json:"lastCheck"`
}

type HealthChecker struct {
    checks map[string]func(context.Context) Check
    mutex  sync.RWMutex
    cache  map[string]Check
}

func NewHealthChecker() *HealthChecker {
    return &HealthChecker{
        checks: make(map[string]func(context.Context) Check),
        cache:  make(map[string]Check),
    }
}

func (hc *HealthChecker) RegisterCheck(name string, check func(context.Context) Check) {
    hc.mutex.Lock()
    defer hc.mutex.Unlock()
    hc.checks[name] = check
}

func (hc *HealthChecker) RunChecks(ctx context.Context) map[string]Check {
    hc.mutex.Lock()
    defer hc.mutex.Unlock()

    results := make(map[string]Check)
    var wg sync.WaitGroup

    for name, check := range hc.checks {
        wg.Add(1)
        go func(name string, check func(context.Context) Check) {
            defer wg.Done()

            checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            defer cancel()

            result := check(checkCtx)
            result.LastCheck = time.Now()

            hc.cache[name] = result
            results[name] = result

            if result.Status == StatusDown {
                logger.Error("Health check failed", nil, logger.Fields{
                    "check": name,
                    "error": result.Error,
                })
            }
        }(name, check)
    }

    wg.Wait()
    return results
}

func (hc *HealthChecker) GetStatus() Status {
    hc.mutex.RLock()
    defer hc.mutex.RUnlock()

    hasDown := false
    hasDegraded := false

    for _, check := range hc.cache {
        switch check.Status {
        case StatusDown:
            hasDown = true
        case StatusDegraded:
            hasDegraded = true
        }
    }

    if hasDown {
        return StatusDown
    }
    if hasDegraded {
        return StatusDegraded
    }
    return StatusUp
} 