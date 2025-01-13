package router

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/handler"
    "github.com/victorradael/condoguard/internal/middleware"
)

type VersionedRouter struct {
    engine *gin.Engine
}

func NewVersionedRouter(engine *gin.Engine) *VersionedRouter {
    return &VersionedRouter{
        engine: engine,
    }
}

func (vr *VersionedRouter) SetupRoutes() {
    // Apply version middleware
    vr.engine.Use(middleware.VersionMiddleware())

    // API group
    api := vr.engine.Group("/api")
    {
        // V1 routes
        v1 := api.Group("/v1")
        {
            vr.setupV1Routes(v1)
        }

        // V2 routes (when needed)
        v2 := api.Group("/v2")
        {
            vr.setupV2Routes(v2)
        }
    }
}

func (vr *VersionedRouter) setupV1Routes(rg *gin.RouterGroup) {
    // Auth routes
    auth := rg.Group("/auth")
    {
        auth.POST("/register", handler.V1Register)
        auth.POST("/login", handler.V1Login)
    }

    // Protected routes
    protected := rg.Group("")
    protected.Use(middleware.AuthMiddleware())
    {
        // User routes
        users := protected.Group("/users")
        {
            users.GET("", handler.V1GetAllUsers)
            users.GET("/:id", handler.V1GetUserByID)
            users.POST("", handler.V1CreateUser)
            users.PUT("/:id", handler.V1UpdateUser)
            users.DELETE("/:id", handler.V1DeleteUser)
        }

        // Other V1 routes...
    }
}

func (vr *VersionedRouter) setupV2Routes(rg *gin.RouterGroup) {
    // V2 specific routes will be implemented here
    // For now, we'll just set up a placeholder
    rg.GET("/status", func(c *gin.Context) {
        c.JSON(200, gin.H{
            "version": "v2",
            "status":  "operational",
        })
    })
} 