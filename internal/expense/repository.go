package expense

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository defines the persistence contract for the expense domain.
type Repository interface {
	Save(ctx context.Context, e *Expense) error
	FindByID(ctx context.Context, id string) (*Expense, error)
	FindAll(ctx context.Context, filter Filter) ([]*Expense, error)
	Update(ctx context.Context, e *Expense) error
	Delete(ctx context.Context, id string) error
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Expense
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Expense)}
}

func (r *inMemoryRepository) Save(_ context.Context, e *Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.ID = bson.NewObjectID().Hex()
	cp := *e
	r.store[e.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context, f Filter) ([]*Expense, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Expense, 0, len(r.store))
	for _, e := range r.store {
		if f.From != nil && e.DueDate.Before(*f.From) {
			continue
		}
		if f.To != nil && e.DueDate.After(*f.To) {
			continue
		}
		cp := *e
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, e *Expense) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[e.ID]; !ok {
		return ErrNotFound
	}
	cp := *e
	r.store[e.ID] = &cp
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

	coll := client.Database(dbName).Collection("expenses")

	// Index on dueDate to support range queries efficiently.
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "dueDate", Value: 1}},
	})
	if err != nil {
		return nil, err
	}

	return &mongoRepository{coll: coll, client: client, dbName: dbName}, nil
}

func (r *mongoRepository) Save(ctx context.Context, e *Expense) error {
	if e.ID == "" {
		e.ID = bson.NewObjectID().Hex()
	}
	_, err := r.coll.InsertOne(ctx, e)
	return err
}

func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Expense, error) {
	var e Expense
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &e, err
}

func (r *mongoRepository) FindAll(ctx context.Context, f Filter) ([]*Expense, error) {
	filter := bson.M{}
	if f.From != nil || f.To != nil {
		due := bson.M{}
		if f.From != nil {
			due["$gte"] = *f.From
		}
		if f.To != nil {
			due["$lte"] = *f.To
		}
		filter["dueDate"] = due
	}

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "dueDate", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []*Expense
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Expense{}
	}
	return list, nil
}

func (r *mongoRepository) Update(ctx context.Context, e *Expense) error {
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": e.ID}, e)
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


