package middleware

import (
    "github.com/gin-gonic/gin"
    "github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
    validate = validator.New()
}

func ValidateRequest(model interface{}) gin.HandlerFunc {
    return func(c *gin.Context) {
        if err := c.ShouldBindJSON(model); err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            c.Abort()
            return
        }

        if err := validate.Struct(model); err != nil {
            c.JSON(400, gin.H{"error": err.Error()})
            c.Abort()
            return
        }

        c.Next()
    }
} 