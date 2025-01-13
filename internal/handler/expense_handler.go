package handler

import (
    "github.com/gin-gonic/gin"
    "github.com/victorradael/condoguard/internal/model"
    "github.com/victorradael/condoguard/internal/repository"
)

func GetAllExpenses(c *gin.Context) {
    repo := repository.NewExpenseRepository()
    expenses, err := repo.FindAll()
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to fetch expenses"})
        return
    }
    c.JSON(200, expenses)
}

func GetExpenseByID(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewExpenseRepository()
    
    expense, err := repo.FindByID(id)
    if err != nil {
        c.JSON(404, gin.H{"error": "Expense not found"})
        return
    }
    
    c.JSON(200, expense)
}

func CreateExpense(c *gin.Context) {
    var expense model.Expense
    if err := c.ShouldBindJSON(&expense); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Validate associated entities if they exist
    if expense.Resident != nil {
        residentRepo := repository.NewResidentRepository()
        if _, err := residentRepo.FindByID(expense.Resident.ID.Hex()); err != nil {
            c.JSON(400, gin.H{"error": "Invalid resident ID"})
            return
        }
    }

    if expense.ShopOwner != nil {
        shopOwnerRepo := repository.NewShopOwnerRepository()
        if _, err := shopOwnerRepo.FindByID(expense.ShopOwner.ID.Hex()); err != nil {
            c.JSON(400, gin.H{"error": "Invalid shop owner ID"})
            return
        }
    }

    repo := repository.NewExpenseRepository()
    createdExpense, err := repo.Create(&expense)
    if err != nil {
        c.JSON(500, gin.H{"error": "Failed to create expense"})
        return
    }

    c.JSON(201, createdExpense)
}

func UpdateExpense(c *gin.Context) {
    id := c.Param("id")
    var expense model.Expense
    if err := c.ShouldBindJSON(&expense); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    repo := repository.NewExpenseRepository()
    if err := repo.Update(id, &expense); err != nil {
        c.JSON(500, gin.H{"error": "Failed to update expense"})
        return
    }

    c.JSON(200, gin.H{"message": "Expense updated successfully"})
}

func DeleteExpense(c *gin.Context) {
    id := c.Param("id")
    repo := repository.NewExpenseRepository()
    
    if err := repo.Delete(id); err != nil {
        c.JSON(500, gin.H{"error": "Failed to delete expense"})
        return
    }

    c.JSON(200, gin.H{"message": "Expense deleted successfully"})
} 