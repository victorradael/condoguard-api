package repository

import (
    "context"
    "github.com/victorradael/condoguard/internal/config"
    "github.com/victorradael/condoguard/internal/model"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type ShopOwnerRepository struct {
    collection *mongo.Collection
}

func NewShopOwnerRepository() *ShopOwnerRepository {
    return &ShopOwnerRepository{
        collection: config.DB.Collection("shopOwners"),
    }
}

func (r *ShopOwnerRepository) Create(shopOwner *model.ShopOwner) (*model.ShopOwner, error) {
    result, err := r.collection.InsertOne(context.Background(), shopOwner)
    if err != nil {
        return nil, err
    }
    shopOwner.ID = result.InsertedID.(primitive.ObjectID)
    return shopOwner, nil
}

func (r *ShopOwnerRepository) FindAll() ([]model.ShopOwner, error) {
    cursor, err := r.collection.Find(context.Background(), bson.M{})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var shopOwners []model.ShopOwner
    if err = cursor.All(context.Background(), &shopOwners); err != nil {
        return nil, err
    }
    return shopOwners, nil
}

func (r *ShopOwnerRepository) FindByID(id string) (*model.ShopOwner, error) {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var shopOwner model.ShopOwner
    err = r.collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&shopOwner)
    if err != nil {
        return nil, err
    }
    return &shopOwner, nil
}

func (r *ShopOwnerRepository) Update(id string, shopOwner *model.ShopOwner) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.UpdateOne(
        context.Background(),
        bson.M{"_id": objectID},
        bson.M{"$set": shopOwner},
    )
    return err
}

func (r *ShopOwnerRepository) Delete(id string) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
    return err
} 