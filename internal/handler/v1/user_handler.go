package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/victorradael/condoguard/internal/model"
	"github.com/victorradael/condoguard/internal/repository"
)

// V1UserHandler handles user-related requests for API v1
type V1UserHandler struct {
	userRepo *repository.UserRepository
}

func NewV1UserHandler(userRepo *repository.UserRepository) *V1UserHandler {
	return &V1UserHandler{
		userRepo: userRepo,
	}
}

func (h *V1UserHandler) GetAllUsers(c *gin.Context) {
	users, err := h.userRepo.FindAll()
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to fetch users"})
		return
	}

	// V1 specific response formatting
	response := make([]map[string]interface{}, len(users))
	for i, user := range users {
		response[i] = map[string]interface{}{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"email":    user.Email,
			"roles":    user.Roles,
		}
	}

	c.JSON(200, response)
}

// Other V1 specific handler methods... 