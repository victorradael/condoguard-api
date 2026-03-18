package notification

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository defines the persistence contract for the notification domain.
type Repository interface {
	Save(ctx context.Context, n *Notification) error
	FindByID(ctx context.Context, id string) (*Notification, error)
	FindAll(ctx context.Context) ([]*Notification, error)
	Update(ctx context.Context, n *Notification) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Notification
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Notification)}
}

func (r *inMemoryRepository) Save(_ context.Context, n *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	n.ID = bson.NewObjectID().Hex()
	cp := *n
	if cp.ResidentIDs == nil {
		cp.ResidentIDs = []string{}
	}
	if cp.ShopOwnerIDs == nil {
		cp.ShopOwnerIDs = []string{}
	}
	r.store[n.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *n
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*Notification, 0, len(r.store))
	for _, n := range r.store {
		cp := *n
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, n *Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[n.ID]; !ok {
		return ErrNotFound
	}
	cp := *n
	r.store[n.ID] = &cp
	return nil
}

func (r *inMemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[id]; !ok {
		return ErrNotFound
	}
	delete(r.store, id)
	return nil
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

func NewMongoRepository(ctx context.Context, uri, dbName string) (MongoRepository, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	coll := client.Database(dbName).Collection("notifications")

	// Index on createdAt for ordered listing.
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "createdAt", Value: -1}},
	})
	if err != nil {
		return nil, err
	}

	return &mongoRepository{coll: coll, client: client, dbName: dbName}, nil
}

func (r *mongoRepository) Save(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = bson.NewObjectID().Hex()
	}
	_, err := r.coll.InsertOne(ctx, n)
	return err
}

func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Notification, error) {
	var n Notification
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&n)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &n, err
}

func (r *mongoRepository) FindAll(ctx context.Context) ([]*Notification, error) {
	cursor, err := r.coll.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []*Notification
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Notification{}
	}
	return list, nil
}

func (r *mongoRepository) Update(ctx context.Context, n *Notification) error {
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": n.ID}, n)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *mongoRepository) Cleanup(ctx context.Context) {
	_ = r.client.Database(r.dbName).Drop(ctx)
	_ = r.client.Disconnect(ctx)
}
