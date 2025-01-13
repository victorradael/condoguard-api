package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/victorradael/condoguard/internal/repository"
)

func GetAllShopOwners(c *gin.Context) {
    repo := repository.NewShopOwnerRepository()
    shopOwners, err := repo.FindAll()
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to fetch shop owners"})
        return
    }
    c.JSON(200, shopOwners)
}

func GetShopOwnerByID(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewShopOwnerRepository()
    
    shopOwner, err := repo.FindByID(id)
    if err != nil {
        c.JSON(404, gin.H{"error": "Shop owner not found"})
        return
    }
    
    c.JSON(200, shopOwner)
}

func CreateShopOwner(c *gin.Context) {
    var shopOwner model.ShopOwner
    if err := c.ShouldBindJSON(&shopOwner); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewShopOwnerRepository()
    createdShopOwner, err := repo.Create(&shopOwner)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to create shop owner"})
        return
    }

    c.JSON(201, createdShopOwner)
}

func UpdateShopOwner(c *gin.Context) {
    id := c.Param("id")
    var shopOwner model.ShopOwner
    if err := c.ShouldBindJSON(&shopOwner); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewShopOwnerRepository()
    if err := repo.Update(id, &shopOwner); err != nil {
        c.JSON(500, gin.H{"error": "Failed to update shop owner"})
        return
    }

    c.JSON(200, gin.H{"message": "Shop owner updated successfully"})
}

func DeleteShopOwner(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewShopOwnerRepository()
    
    if err := repo.Delete(id); err != nil {
        c.JSON(500, gin.H{"error": "Failed to delete shop owner"})
        return
    }

    c.JSON(200, gin.H{"message": "Shop owner deleted successfully"})
} 