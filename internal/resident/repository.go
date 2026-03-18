package resident

import (
	"context"
	"errors"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository defines the persistence contract for the resident domain.
type Repository interface {
	Save(ctx context.Context, r *Resident) error
	FindByID(ctx context.Context, id string) (*Resident, error)
	FindAll(ctx context.Context) ([]*Resident, error)
	Update(ctx context.Context, r *Resident) error
	Delete(ctx context.Context, id string) error
	ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error)
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type inMemoryRepository struct {
	mu    sync.RWMutex
	store map[string]*Resident
}

func NewInMemoryRepository() Repository {
	return &inMemoryRepository{store: make(map[string]*Resident)}
}

func (r *inMemoryRepository) Save(_ context.Context, res *Resident) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.store {
		if existing.CondominiumID == res.CondominiumID && existing.UnitNumber == res.UnitNumber {
			return ErrDuplicate
		}
	}

	res.ID = bson.NewObjectID().Hex()
	cp := *res
	r.store[res.ID] = &cp
	return nil
}

func (r *inMemoryRepository) FindByID(_ context.Context, id string) (*Resident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res, ok := r.store[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *res
	return &cp, nil
}

func (r *inMemoryRepository) FindAll(_ context.Context) ([]*Resident, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*Resident, 0, len(r.store))
	for _, res := range r.store {
		cp := *res
		list = append(list, &cp)
	}
	return list, nil
}

func (r *inMemoryRepository) Update(_ context.Context, res *Resident) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.store[res.ID]; !ok {
		return ErrNotFound
	}
	cp := *res
	r.store[res.ID] = &cp
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

func (r *inMemoryRepository) ExistsUnit(_ context.Context, condominiumID, unitNumber, excludeID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, res := range r.store {
		if res.ID == excludeID {
			continue
		}
		if res.CondominiumID == condominiumID && res.UnitNumber == unitNumber {
			return true, nil
		}
	}
	return false, nil
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

	coll := client.Database(dbName).Collection("residents")

	// Compound unique index: unitNumber + condominiumId
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "condominiumId", Value: 1}, {Key: "unitNumber", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return nil, err
	}

	return &mongoRepository{coll: coll, client: client, dbName: dbName}, nil
}

func (r *mongoRepository) Save(ctx context.Context, res *Resident) error {
	if res.ID == "" {
		res.ID = bson.NewObjectID().Hex()
	}
	_, err := r.coll.InsertOne(ctx, res)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	return err
}

func (r *mongoRepository) FindByID(ctx context.Context, id string) (*Resident, error) {
	var res Resident
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&res)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	return &res, err
}

func (r *mongoRepository) FindAll(ctx context.Context) ([]*Resident, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var list []*Resident
	if err := cursor.All(ctx, &list); err != nil {
		return nil, err
	}
	if list == nil {
		list = []*Resident{}
	}
	return list, nil
}

func (r *mongoRepository) Update(ctx context.Context, res *Resident) error {
	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": res.ID}, res)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
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

func (r *mongoRepository) ExistsUnit(ctx context.Context, condominiumID, unitNumber, excludeID string) (bool, error) {
	filter := bson.M{
		"condominiumId": condominiumID,
		"unitNumber":    unitNumber,
	}
	if excludeID != "" {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	count, err := r.coll.CountDocuments(ctx, filter)
	return count > 0, err
}

func (r *mongoRepository) Cleanup(ctx context.Context) {
	_ = r.client.Database(r.dbName).Drop(ctx)
	_ = r.client.Disconnect(ctx)
}
