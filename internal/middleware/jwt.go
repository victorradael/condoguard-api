package middleware

import (
    "fmt"
    "os"
    "time"
    "github.com/golang-jwt/jwt/v5"
    "github.com/gin-gonic/gin"
)

func GenerateToken(username string) (string, error) {
    secret := []byte(os.Getenv("JWT_SECRET_KEY"))
    
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "username": username,
        "exp":     time.Now().Add(time.Hour * 24).Unix(),
    })

    return token.SignedString(secret)
}

func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        tokenString := c.GetHeader("Authorization")
        if tokenString == "" {
            c.JSON(401, gin.H{"error": "Authorization header required"})
            c.Abort()
            return
        }

        // Remove "Bearer " prefix if present
        if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
            tokenString = tokenString[7:]
        }

        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return []byte(os.Getenv("JWT_SECRET_KEY")), nil
        })

        if err != nil {
            c.JSON(401, gin.H{"error": "Invalid token"})
            c.Abort()
            return
        }

        if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
            c.Set("username", claims["username"])
            c.Next()
        } else {
            c.JSON(401, gin.H{"error": "Invalid token claims"})
            c.Abort()
            return
        }
    }
} 