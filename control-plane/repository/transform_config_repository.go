package repository

import (
	"context"
	"errors"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionTransformConfigs = "transform_configurations"

type TransformResponseConfigRepository struct {
	db *mongo.Database
}

func NewTransformConfigRepository(db *mongo.Database) *TransformResponseConfigRepository {
	return &TransformResponseConfigRepository{db: db}
}

func (r *TransformResponseConfigRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.db.Collection(collectionTransformConfigs).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "serviceId", Value: 1}, {Key: "deletedAt", Value: 1}}},
		{Keys: bson.D{{Key: "enabled", Value: 1}, {Key: "deletedAt", Value: 1}}},
	})
	return err
}

func (r *TransformResponseConfigRepository) Create(ctx context.Context, cfg *model.TransformConfig, adminID bson.ObjectID) error {
	now := time.Now().UTC()
	cfg.ID = bson.NewObjectID()
	cfg.CreatedAt = now
	cfg.CreatedBy = &adminID
	_, err := r.db.Collection(collectionTransformConfigs).InsertOne(ctx, cfg)
	return err
}

func (r *TransformResponseConfigRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.TransformConfig, error) {
	var cfg model.TransformConfig
	err := r.db.Collection(collectionTransformConfigs).FindOne(ctx, bson.M{
		"_id":       id,
		"deletedAt": bson.M{"$exists": false},
	}).Decode(&cfg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &cfg, err
}

func (r *TransformResponseConfigRepository) FindByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]*model.TransformConfig, error) {
	cursor, err := r.db.Collection(collectionTransformConfigs).Find(ctx,
		bson.M{"serviceId": serviceID, "deletedAt": bson.M{"$exists": false}},
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var cfgs []*model.TransformConfig
	return cfgs, cursor.All(ctx, &cfgs)
}

func (r *TransformResponseConfigRepository) FindAllActive(ctx context.Context) ([]*model.TransformConfig, error) {
	cursor, err := r.db.Collection(collectionTransformConfigs).Find(ctx,
		bson.M{"enabled": true, "deletedAt": bson.M{"$exists": false}},
		options.Find().SetSort(bson.D{{Key: "serviceId", Value: 1}, {Key: "createdAt", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var cfgs []*model.TransformConfig
	return cfgs, cursor.All(ctx, &cfgs)
}

func (r *TransformResponseConfigRepository) List(ctx context.Context, serviceID *bson.ObjectID, limit, offset int64) ([]*model.TransformConfig, int64, error) {
	filter := bson.M{"deletedAt": bson.M{"$exists": false}}
	if serviceID != nil {
		filter["serviceId"] = *serviceID
	}

	total, err := r.db.Collection(collectionTransformConfigs).CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	cursor, err := r.db.Collection(collectionTransformConfigs).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(limit).SetSkip(offset),
	)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var cfgs []*model.TransformConfig
	return cfgs, total, cursor.All(ctx, &cfgs)
}

func (r *TransformResponseConfigRepository) Update(ctx context.Context, id bson.ObjectID, req *model.TransformConfigRequest, adminID bson.ObjectID) error {
	now := time.Now().UTC()
	serviceID, err := bson.ObjectIDFromHex(req.ServiceID)
	if err != nil {
		return err
	}
	_, err = r.db.Collection(collectionTransformConfigs).UpdateOne(ctx,
		bson.M{"_id": id, "deletedAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{
			"name":        req.Name,
			"description": req.Description,
			"serviceId":   serviceID,
			"match":       req.Match,
			"enabled":     req.Enabled,
			"rules":       req.Rules,
			"updatedAt":   now,
			"updatedBy":   adminID,
		}},
	)
	return err
}

func (r *TransformResponseConfigRepository) Delete(ctx context.Context, id bson.ObjectID, adminID bson.ObjectID) error {
	now := time.Now().UTC()
	_, err := r.db.Collection(collectionTransformConfigs).UpdateOne(ctx,
		bson.M{"_id": id, "deletedAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"deletedAt": now, "deletedBy": adminID}},
	)
	return err
}
