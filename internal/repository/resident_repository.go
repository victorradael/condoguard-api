package repository

import (
    "context"
    "github.com/victorradael/condoguard/internal/config"
    "github.com/victorradael/condoguard/internal/model"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type ResidentRepository struct {
    collection *mongo.Collection
}

func NewResidentRepository() *ResidentRepository {
    return &ResidentRepository{
        collection: config.DB.Collection("residents"),
    }
}

func (r *ResidentRepository) Create(resident *model.Resident) (*model.Resident, error) {
    result, err := r.collection.InsertOne(context.Background(), resident)
    if err != nil {
        return nil, err
    }
    resident.ID = result.InsertedID.(primitive.ObjectID)
    return resident, nil
}

func (r *ResidentRepository) FindAll() ([]model.Resident, error) {
    cursor, err := r.collection.Find(context.Background(), bson.M{})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var residents []model.Resident
    if err = cursor.All(context.Background(), &residents); err != nil {
        return nil, err
    }
    return residents, nil
}

func (r *ResidentRepository) FindByID(id string) (*model.Resident, error) {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var resident model.Resident
    err = r.collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&resident)
    if err != nil {
        return nil, err
    }
    return &resident, nil
}

func (r *ResidentRepository) Update(id string, resident *model.Resident) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.UpdateOne(
        context.Background(),
        bson.M{"_id": objectID},
        bson.M{"$set": resident},
    )
    return err
}

func (r *ResidentRepository) Delete(id string) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
    return err
} 