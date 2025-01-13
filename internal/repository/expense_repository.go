package repository

import (
    "context"
    "github.com/victorradael/condoguard/internal/config"
    "github.com/victorradael/condoguard/internal/model"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type ExpenseRepository struct {
    collection *mongo.Collection
}

func NewExpenseRepository() *ExpenseRepository {
    return &ExpenseRepository{
        collection: config.DB.Collection("expenses"),
    }
}

func (r *ExpenseRepository) Create(expense *model.Expense) (*model.Expense, error) {
    result, err := r.collection.InsertOne(context.Background(), expense)
    if err != nil {
        return nil, err
    }
    expense.ID = result.InsertedID.(primitive.ObjectID)
    return expense, nil
}

func (r *ExpenseRepository) FindAll() ([]model.Expense, error) {
    cursor, err := r.collection.Find(context.Background(), bson.M{})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var expenses []model.Expense
    if err = cursor.All(context.Background(), &expenses); err != nil {
        return nil, err
    }
    return expenses, nil
}

func (r *ExpenseRepository) FindByID(id string) (*model.Expense, error) {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var expense model.Expense
    err = r.collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&expense)
    if err != nil {
        return nil, err
    }
    return &expense, nil
}

func (r *ExpenseRepository) Update(id string, expense *model.Expense) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.UpdateOne(
        context.Background(),
        bson.M{"_id": objectID},
        bson.M{"$set": expense},
    )
    return err
}

func (r *ExpenseRepository) Delete(id string) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
    return err
} 