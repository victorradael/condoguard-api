package middleware

import (
    "github.com/gin-gonic/gin"
    "strings"
)

type APIVersion string

const (
    V1 APIVersion = "v1"
    V2 APIVersion = "v2"
)

// VersionMiddleware handles API versioning through URL path or Accept header
func VersionMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        var version APIVersion

        // Check URL path version first
        path := strings.Split(c.Request.URL.Path, "/")
        for i, segment := range path {
            if strings.HasPrefix(segment, "v") {
                version = APIVersion(segment)
                // Remove version from path
                path = append(path[:i], path[i+1:]...)
                c.Request.URL.Path = strings.Join(path, "/")
                break
            }
        }

        // If no version in URL, check Accept header
        if version == "" {
            accept := c.GetHeader("Accept")
            if strings.Contains(accept, "application/vnd.condoguard.") {
                parts := strings.Split(accept, ".")
                version = APIVersion(parts[len(parts)-1])
            }
        }

        // Default to V1 if no version specified
        if version == "" {
            version = V1
        }

        c.Set("api_version", version)
        c.Next()
    }
} 