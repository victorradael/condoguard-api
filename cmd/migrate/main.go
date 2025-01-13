package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/joho/godotenv"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "go.mongodb.org/mongo-driver/bson"
)

func main() {
    if err := godotenv.Load(); err != nil {
        log.Printf("Warning: .env file not found")
    }

    // Connect to MongoDB
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    client, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("MONGODB_URI")))
    if err != nil {
        log.Fatal(err)
    }
    defer client.Disconnect(ctx)

    db := client.Database("condoguard")

    // Create indexes
    createIndexes(ctx, db)

    log.Println("Migration completed successfully!")
}

func createIndexes(ctx context.Context, db *mongo.Database) {
    // Users collection indexes
    usersColl := db.Collection("users")
    _, err := usersColl.Indexes().CreateMany(ctx, []mongo.IndexModel{
        {
            Keys: bson.D{{Key: "username", Value: 1}},
            Options: options.Index().SetUnique(true),
        },
        {
            Keys: bson.D{{Key: "email", Value: 1}},
            Options: options.Index().SetUnique(true),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Residents collection indexes
    residentsColl := db.Collection("residents")
    _, err = residentsColl.Indexes().CreateOne(ctx, mongo.IndexModel{
        Keys: bson.D{{Key: "unitNumber", Value: 1}},
        Options: options.Index().SetUnique(true),
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Indexes created successfully!")
} 