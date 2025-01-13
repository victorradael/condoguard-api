package performance

import (
    "context"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

// QueryOptimizer provides methods for optimizing database queries
type QueryOptimizer struct {
    db *mongo.Database
}

func NewQueryOptimizer(db *mongo.Database) *QueryOptimizer {
    return &QueryOptimizer{db: db}
}

// CreateIndexes creates optimal indexes for collections
func (qo *QueryOptimizer) CreateIndexes(ctx context.Context) error {
    // Users collection indexes
    if err := qo.createUserIndexes(ctx); err != nil {
        return err
    }

    // Residents collection indexes
    if err := qo.createResidentIndexes(ctx); err != nil {
        return err
    }

    // Expenses collection indexes
    if err := qo.createExpenseIndexes(ctx); err != nil {
        return err
    }

    // Notifications collection indexes
    if err := qo.createNotificationIndexes(ctx); err != nil {
        return err
    }

    return nil
}

func (qo *QueryOptimizer) createUserIndexes(ctx context.Context) error {
    userIndexes := []mongo.IndexModel{
        {
            Keys: bson.D{
                {Key: "username", Value: 1},
            },
            Options: options.Index().SetUnique(true),
        },
        {
            Keys: bson.D{
                {Key: "email", Value: 1},
            },
            Options: options.Index().SetUnique(true),
        },
    }

    _, err := qo.db.Collection("users").Indexes().CreateMany(ctx, userIndexes)
    return err
}

func (qo *QueryOptimizer) createResidentIndexes(ctx context.Context) error {
    residentIndexes := []mongo.IndexModel{
        {
            Keys: bson.D{
                {Key: "unitNumber", Value: 1},
            },
            Options: options.Index().SetUnique(true),
        },
        {
            Keys: bson.D{
                {Key: "owner._id", Value: 1},
            },
        },
    }

    _, err := qo.db.Collection("residents").Indexes().CreateMany(ctx, residentIndexes)
    return err
}

func (qo *QueryOptimizer) createExpenseIndexes(ctx context.Context) error {
    expenseIndexes := []mongo.IndexModel{
        {
            Keys: bson.D{
                {Key: "date", Value: -1},
            },
        },
        {
            Keys: bson.D{
                {Key: "resident._id", Value: 1},
                {Key: "date", Value: -1},
            },
        },
    }

    _, err := qo.db.Collection("expenses").Indexes().CreateMany(ctx, expenseIndexes)
    return err
}

func (qo *QueryOptimizer) createNotificationIndexes(ctx context.Context) error {
    notificationIndexes := []mongo.IndexModel{
        {
            Keys: bson.D{
                {Key: "createdAt", Value: -1},
            },
        },
        {
            Keys: bson.D{
                {Key: "createdBy._id", Value: 1},
            },
        },
    }

    _, err := qo.db.Collection("notifications").Indexes().CreateMany(ctx, notificationIndexes)
    return err
} 