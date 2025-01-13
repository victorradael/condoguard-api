package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/stretchr/testify/assert"
)

func TestAuthFlow(t *testing.T) {
    // Initialize test server and client
    router := setupTestRouter()
    server := httptest.NewServer(router)
    defer server.Close()

    // Test Registration
    t.Run("Registration Flow", func(t *testing.T) {
        user := model.User{
            Username: "testuser",
            Password: "testpass",
            Email:    "test@example.com",
            Roles:    []string{"USER"},
        }

        body, _ := json.Marshal(user)
        resp, err := http.Post(server.URL+"/api/auth/register", "application/json", bytes.NewBuffer(body))
        assert.NoError(t, err)
        assert.Equal(t, http.StatusCreated, resp.StatusCode)
    })

    // Test Login
    t.Run("Login Flow", func(t *testing.T) {
        credentials := model.AuthRequest{
            Username: "testuser",
            Password: "testpass",
        }

        body, _ := json.Marshal(credentials)
        resp, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewBuffer(body))
        assert.NoError(t, err)
        assert.Equal(t, http.StatusOK, resp.StatusCode)

        var response map[string]interface{}
        json.NewDecoder(resp.Body).Decode(&response)
        assert.Contains(t, response, "token")
    })
} 