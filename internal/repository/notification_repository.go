package repository

import (
    "context"
    "github.com/victorradael/condoguard/internal/config"
    "github.com/victorradael/condoguard/internal/model"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type NotificationRepository struct {
    collection *mongo.Collection
}

func NewNotificationRepository() *NotificationRepository {
    return &NotificationRepository{
        collection: config.DB.Collection("notifications"),
    }
}

func (r *NotificationRepository) Create(notification *model.Notification) (*model.Notification, error) {
    result, err := r.collection.InsertOne(context.Background(), notification)
    if err != nil {
        return nil, err
    }
    notification.ID = result.InsertedID.(primitive.ObjectID)
    return notification, nil
}

func (r *NotificationRepository) FindAll() ([]model.Notification, error) {
    cursor, err := r.collection.Find(context.Background(), bson.M{})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var notifications []model.Notification
    if err = cursor.All(context.Background(), &notifications); err != nil {
        return nil, err
    }
    return notifications, nil
}

func (r *NotificationRepository) FindByID(id string) (*model.Notification, error) {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return nil, err
    }

    var notification model.Notification
    err = r.collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&notification)
    if err != nil {
        return nil, err
    }
    return &notification, nil
}

func (r *NotificationRepository) Update(id string, notification *model.Notification) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.UpdateOne(
        context.Background(),
        bson.M{"_id": objectID},
        bson.M{"$set": notification},
    )
    return err
}

func (r *NotificationRepository) Delete(id string) error {
    objectID, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return err
    }

    _, err = r.collection.DeleteOne(context.Background(), bson.M{"_id": objectID})
    return err
} 