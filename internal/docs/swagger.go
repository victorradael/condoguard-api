package docs

import "github.com/swaggo/swag"

// @title CondoGuard API
// @version 2.0
// @description API for condominium management system with versioning support
// @host localhost:8080
// @BasePath /api
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization

// @x-version-mapping
// @x-version v1 /api/v1
// @x-version v2 /api/v2

// @x-version-info
// @x-info v1 Stable version with basic functionality
// @x-info v2 Beta version with advanced features

type swaggerInfo struct{} 