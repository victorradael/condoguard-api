package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/victorradael/condoguard/internal/repository"
    "time"
)

func GetAllNotifications(c *gin.Context) {
    repo := repository.NewNotificationRepository()
    notifications, err := repo.FindAll()
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to fetch notifications"})
        return
    }
    c.JSON(200, notifications)
}

func GetNotificationByID(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewNotificationRepository()
    
    notification, err := repo.FindByID(id)
    if err != nil {
        c.JSON(404, gin.H{"error": "Notification not found"})
        return
    }
    
    c.JSON(200, notification)
}

func CreateNotification(c *gin.Context) {
    var notification model.Notification
    if err := c.ShouldBindJSON(&notification); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Set creation time
    notification.CreatedAt = time.Now()

    repo := repository.NewNotificationRepository()
    createdNotification, err := repo.Create(&notification)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to create notification"})
        return
    }

    c.JSON(201, createdNotification)
}

func UpdateNotification(c *gin.Context) {
    id := c.Param("id")
    var notification model.Notification
    if err := c.ShouldBindJSON(&notification); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewNotificationRepository()
    if err := repo.Update(id, &notification); err != nil {
        c.JSON(500, gin.H{"error": "Failed to update notification"})
        return
    }

    c.JSON(200, gin.H{"message": "Notification updated successfully"})
}

func DeleteNotification(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewNotificationRepository()
    
    if err := repo.Delete(id); err != nil {
        c.JSON(500, gin.H{"error": "Failed to delete notification"})
        return
    }

    c.JSON(200, gin.H{"message": "Notification deleted successfully"})
} 