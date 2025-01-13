package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/victorradael/condoguard/internal/repository"
)

func GetAllResidents(c *gin.Context) {
    repo := repository.NewResidentRepository()
    residents, err := repo.FindAll()
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to fetch residents"})
        return
    }
    c.JSON(200, residents)
}

func GetResidentByID(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewResidentRepository()
    
    resident, err := repo.FindByID(id)
    if err != nil {
        c.JSON(404, gin.H{"error": "Resident not found"})
        return
    }
    
    c.JSON(200, resident)
}

func CreateResident(c *gin.Context) {
    var resident model.Resident
    if err := c.ShouldBindJSON(&resident); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewResidentRepository()
    createdResident, err := repo.Create(&resident)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to create resident"})
        return
    }

    c.JSON(201, createdResident)
}

func UpdateResident(c *gin.Context) {
    id := c.Param("id")
    var resident model.Resident
    if err := c.ShouldBindJSON(&resident); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewResidentRepository()
    if err := repo.Update(id, &resident); err != nil {
        c.JSON(500, gin.H{"error": "Failed to update resident"})
        return
    }

    c.JSON(200, gin.H{"message": "Resident updated successfully"})
}

func DeleteResident(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewResidentRepository()
    
    if err := repo.Delete(id); err != nil {
        c.JSON(500, gin.H{"error": "Failed to delete resident"})
        return
    }

    c.JSON(200, gin.H{"message": "Resident deleted successfully"})
} 