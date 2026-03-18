package auth

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrNotFound is returned when a user cannot be found.
var ErrNotFound = errors.New("auth: user not found")

// ErrDuplicateEmail is returned when the e-mail already exists.
var ErrDuplicateEmail = errors.New("auth: email already registered")

// Repository defines the persistence contract for the auth domain.
// Interfaces are defined on the consumer side (idiomatic Go).
type Repository interface {
	Save(ctx context.Context, user *User) error
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
}

// ── In-memory (unit / integration without DB) ─────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*User // keyed by username
}

// NewInMemoryRepository returns a thread-safe in-memory Repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*User)}
}

func (r *inMemoryRepository) Save(_ context.Context, user *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.store {
		if u.Email == user.Email {
			return ErrDuplicateEmail
		}
	}

	if user.ID == "" {
		user.ID = user.Username // simple deterministic ID for tests
	}
	r.store[user.Username] = user
	return nil
}

func (r *inMemoryRepository) FindByUsername(_ context.Context, username string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.store[username]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (r *inMemoryRepository) FindByEmail(_ context.Context, email string) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.store {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, ErrNotFound
}

// ── MongoDB ───────────────────────────────────────────────────────────────────

type mongoRepository struct {
	coll   *mongo.Collection
	client *mongo.Client
	dbName string
}

// MongoRepository extends Repository with test-cleanup capability.
type MongoRepository interface {
	Repository
	Cleanup(ctx context.Context)
}

// NewMongoRepository connects to MongoDB and returns a MongoRepository.
func NewMongoRepository(ctx context.Context, uri, dbName string) (MongoRepository, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	coll := client.Database(dbName).Collection("users")

	// Unique index on email.
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}

	return &mongoRepository{coll: coll, client: client, dbName: dbName}, nil
}

func (r *mongoRepository) Save(ctx context.Context, user *User) error {
	if user.ID == "" {
		user.ID = bson.NewObjectID().Hex()
	}
	_, err := r.coll.InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEmail
	}
	return err
}

func (r *mongoRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.coll.FindOne(ctx, bson.M{"username": username}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (r *mongoRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (r *mongoRepository) Cleanup(ctx context.Context) {
	_ = r.client.Database(r.dbName).Drop(ctx)
	_ = r.client.Disconnect(ctx)
}
