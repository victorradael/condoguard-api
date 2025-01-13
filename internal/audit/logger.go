package audit

import (
    "context"
    "time"
    "github.com/victorradael/condoguard/internal/logger"
    "go.mongodb.org/mongo-driver/bson/primitive"
    "go.mongodb.org/mongo-driver/mongo"
)

type AuditLog struct {
    ID        primitive.ObjectID `bson:"_id,omitempty"`
    UserID    string            `bson:"userId"`
    Action    string            `bson:"action"`
    Resource  string            `bson:"resource"`
    Details   interface{}       `bson:"details"`
    IP        string            `bson:"ip"`
    Timestamp time.Time         `bson:"timestamp"`
}

type AuditLogger struct {
    collection *mongo.Collection
}

func NewAuditLogger(db *mongo.Database) *AuditLogger {
    return &AuditLogger{
        collection: db.Collection("audit_logs"),
    }
}

func (a *AuditLogger) Log(ctx context.Context, entry AuditLog) error {
    entry.Timestamp = time.Now()
    entry.ID = primitive.NewObjectID()

    _, err := a.collection.InsertOne(ctx, entry)
    if err != nil {
        logger.Error("Failed to insert audit log", err, logger.Fields{
            "action":   entry.Action,
            "resource": entry.Resource,
            "userId":   entry.UserID,
        })
        return err
    }

    return nil
}

func (a *AuditLogger) QueryLogs(ctx context.Context, filter interface{}, options *mongo.FindOptions) ([]AuditLog, error) {
    cursor, err := a.collection.Find(ctx, filter, options)
    if err != nil {
        return nil, err
    }
    defer cursor.Close(ctx)

    var logs []AuditLog
    if err = cursor.All(ctx, &logs); err != nil {
        return nil, err
    }

    return logs, nil
} 