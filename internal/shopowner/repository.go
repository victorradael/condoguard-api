package shopowner

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository defines the persistence contract for the shopowner domain.
type Repository interface {
	Save(ctx context.Context, s *ShopOwner) error
	FindByID(ctx context.Context, id string) (*ShopOwner, error)
	FindAll(ctx context.Context) ([]*ShopOwner, error)
	Update(ctx context.Context, s *ShopOwner) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*ShopOwner
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*ShopOwner)}
}

func (r *inMemoryRepository) Save(_ context.Context, s *ShopOwner) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.CNPJ == s.CNPJ {
			return ErrDuplicate
		}
	}

	s.ID = bson.NewObjectID().Hex()
	cp := *s
	r.store[s.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*ShopOwner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*ShopOwner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ShopOwner, 0, len(r.store))
	for _, s := range r.store {
		cp := *s
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, s *ShopOwner) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[s.ID]; !ok {
		return ErrNotFound
	}
	cp := *s
	r.store[s.ID] = &cp
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

	coll := client.Database(dbName).Collection("shopOwners")

	// Unique index on CNPJ (already formatted before save).
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "cnpj", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}

	return &mongoRepository{coll: coll, client: client, dbName: dbName}, nil
}

func (r *mongoRepository) Save(ctx context.Context, s *ShopOwner) error {
	if s.ID == "" {
		s.ID = bson.NewObjectID().Hex()
	}
	_, err := r.coll.InsertOne(ctx, s)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	return err
}

func (r *mongoRepository) FindByID(ctx context.Context, id string) (*ShopOwner, error) {
	var s ShopOwner
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &s, err
}

func (r *mongoRepository) FindAll(ctx context.Context) ([]*ShopOwner, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []*ShopOwner
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*ShopOwner{}
	}
	return list, nil
}

func (r *mongoRepository) Update(ctx context.Context, s *ShopOwner) error {
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": s.ID}, s)
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
