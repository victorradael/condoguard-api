package middleware

import (
    "github.com/gin-gonic/gin"
    "log"
)

type AppError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func ErrorHandler() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()

        // Only handle errors if there are any
        if len(c.Errors) > 0 {
            // Log the error
            log.Printf("Error: %v", c.Errors.Last().Err)

            // Get the last error
            err := c.Errors.Last().Err

            // Check if it's our custom error type
            if appErr, ok := err.(*AppError); ok {
                c.JSON(appErr.Code, gin.H{
                    "error": appErr.Message,
                })
                return
            }

            // Handle unknown errors
            c.JSON(500, gin.H{
                "error": "Internal Server Error",
            })
        }
    }
} 