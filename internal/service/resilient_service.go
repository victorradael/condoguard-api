package service

import (
    "context"
    "github.com/victorradael/condoguard/internal/circuitbreaker"
    "github.com/victorradael/condoguard/internal/logger"
    "time"
)

type ResilientService struct {
    breaker *circuitbreaker.CircuitBreaker
    name    string
}

func NewResilientService(name string) *ResilientService {
    return &ResilientService{
        name: name,
        breaker: circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{
            FailureThreshold: 5,
            ResetTimeout:    30 * time.Second,
            HalfOpenMaxCalls: 3,
        }),
    }
}

func (s *ResilientService) Execute(ctx context.Context, operation func() error) error {
    err := s.breaker.Execute(func() error {
        done := make(chan error, 1)
        go func() {
            done <- operation()
        }()

        select {
        case err := <-done:
            return err
        case <-ctx.Done():
            return ctx.Err()
        }
    })

    if err != nil {
        logger.Error("Service execution failed", err, logger.Fields{
            "service": s.name,
            "state":   s.breaker.GetState(),
        })
    }

    return err
} 