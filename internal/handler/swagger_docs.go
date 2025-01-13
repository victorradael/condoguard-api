package handler

// @title CondoGuard API Documentation
// @version 1.0
// @description API documentation for the CondoGuard condominium management system

// Auth endpoints documentation

// Register godoc
// @Summary Register a new user
// @Description Register a new user in the system
// @Tags auth
// @Accept json
// @Produce json
// @Param user body model.User true "User registration details"
// @Success 201 {object} map[string]interface{} "User registered successfully"
// @Failure 400 {object} map[string]string "Invalid input"
// @Failure 500 {object} map[string]string "Server error"
// @Router /auth/register [post]

// Login godoc
// @Summary Login user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body model.AuthRequest true "Login credentials"
// @Success 200 {object} map[string]interface{} "Login successful"
// @Failure 401 {object} map[string]string "Invalid credentials"
// @Failure 500 {object} map[string]string "Server error"
// @Router /auth/login [post]

// User endpoints documentation

// GetAllUsers godoc
// @Summary Get all users
// @Description Retrieve all users in the system
// @Tags users
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {array} model.User "List of users"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Server error"
// @Router /users [get]

// Resident endpoints documentation

// GetAllResidents godoc
// @Summary Get all residents
// @Description Retrieve all residents in the system
// @Tags residents
// @Accept json
// @Produce json
// @Security Bearer
// @Success 200 {array} model.Resident "List of residents"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Server error"
// @Router /residents [get] 