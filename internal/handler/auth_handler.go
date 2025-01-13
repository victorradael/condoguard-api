package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/victorradael/condoguard/internal/middleware"
    "github.com/victorradael/condoguard/internal/repository"
    "golang.org/x/crypto/bcrypt"
)

func Register(c *gin.Context) {
    var user model.User
    if err := c.ShouldBindJSON(&user); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Hash password
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to hash password"})
        return
    }
    user.Password = string(hashedPassword)

    // Set default role if none provided
    if len(user.Roles) == 0 {
        user.Roles = []string{"USER"}
    }

    userRepo := repository.NewUserRepository()
    createdUser, err := userRepo.Create(&user)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to create user"})
        return
    }

    c.JSON(201, gin.H{
        "message": "User registered successfully",
        "user":    createdUser,
    })
}

func Login(c *gin.Context) {
    var auth model.AuthRequest
    if err := c.ShouldBindJSON(&auth); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    userRepo := repository.NewUserRepository()
    user, err := userRepo.FindByUsername(auth.Username)
    if err != nil {
        c.JSON(401, gin.H{"error": "Invalid credentials"})
        return
    }

    // Check password
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(auth.Password)); err != nil {
        c.JSON(401, gin.H{"error": "Invalid credentials"})
        return
    }

    // Generate token
    token, err := middleware.GenerateToken(user.Username)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to generate token"})
        return
    }

    c.JSON(200, gin.H{
        "token": token,
        "user":  user,
    })
} 