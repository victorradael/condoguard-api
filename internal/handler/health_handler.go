package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/health"
    "net/http"
)

type HealthHandler struct {
    checker *health.HealthChecker
}

func NewHealthHandler(checker *health.HealthChecker) *HealthHandler {
    return &HealthHandler{
        checker: checker,
    }
}

// HealthCheck godoc
// @Summary Get system health status
// @Description Get detailed health status of all system components
// @Tags health
// @Produce json
// @Success 200 {object} map[string]health.Check
// @Failure 503 {object} map[string]health.Check
// @Router /health [get]
func (h *HealthHandler) HealthCheck(c *gin.Context) {
    checks := h.checker.RunChecks(c.Request.Context())
    status := h.checker.GetStatus()

    statusCode := http.StatusOK
    if status == health.StatusDown {
        statusCode = http.StatusServiceUnavailable
    }

    c.JSON(statusCode, gin.H{
        "status":  status,
        "checks":  checks,
        "version": "1.0.0", // TODO: Get from config
    })
}

// LivenessCheck godoc
// @Summary Get system liveness status
// @Description Quick check to determine if the system is alive
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /health/live [get]
func (h *HealthHandler) LivenessCheck(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "status": "UP",
    })
}

// ReadinessCheck godoc
// @Summary Get system readiness status
// @Description Check if the system is ready to accept traffic
// @Tags health
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /health/ready [get]
func (h *HealthHandler) ReadinessCheck(c *gin.Context) {
    status := h.checker.GetStatus()
    statusCode := http.StatusOK

    if status != health.StatusUp {
        statusCode = http.StatusServiceUnavailable
    }

    c.JSON(statusCode, gin.H{
        "status": status,
    })
} 