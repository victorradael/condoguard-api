package repository

import (
    "context"
    "github.com/victorradael/condoguard/internal/config"
    "github.com/victorradael/condoguard/internal/model"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type UserRepository struct {
    collection *mongo.Collection
}

func NewUserRepository() *UserRepository {
    return &UserRepository{
        collection: config.DB.Collection("users"),
    }
}

func (r *UserRepository) FindByUsername(username string) (*model.User, error) {
    var user model.User
    err := r.collection.FindOne(context.Background(), bson.M{"username": username}).Decode(&user)
    if err != nil {
        return nil, err
    }
    return &user, nil
}

func (r *UserRepository) Create(user *model.User) (*model.User, error) {
    result, err := r.collection.InsertOne(context.Background(), user)
    if err != nil {
        return nil, err
    }
    user.ID = result.InsertedID.(primitive.ObjectID)
    return user, nil
}

func (r *UserRepository) FindAll() ([]model.User, error) {
    cursor, err := r.collection.Find(context.Background(), bson.M{})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var users []model.User
    if err = cursor.All(context.Background(), &users); err != nil {
        return nil, err
    }
    return users, nil
}

func (r *UserRepository) FindByID(id string) (*model.User, error) {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var user model.User
    err = r.collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&user)
    if err != nil {
        return nil, err
    }
    return &user, nil
} 