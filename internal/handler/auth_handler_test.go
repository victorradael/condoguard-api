package handler

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
    // Set Gin to Test Mode
    gin.SetMode(gin.TestMode)

    // Setup router
    r := gin.Default()
    r.POST("/register", Register)

    // Test case 1: Successful registration
    t.Run("Successful Registration", func(t *testing.T) {
        user := model.User{
            Username: "testuser",
            Password: "testpass",
            Email:    "test@example.com",
            Roles:    []string{"USER"},
        }

        body, _ := json.Marshal(user)
        req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
        w := httptest.NewRecorder()
        r.ServeHTTP(w, req)

        assert.Equal(t, http.StatusCreated, w.Code)
    })

    // Test case 2: Invalid input
    t.Run("Invalid Registration Input", func(t *testing.T) {
        user := model.User{
            Username: "", // Invalid: empty username
            Password: "testpass",
        }

        body, _ := json.Marshal(user)
        req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
        w := httptest.NewRecorder()
        r.ServeHTTP(w, req)

        assert.Equal(t, http.StatusBadRequest, w.Code)
    })
} 