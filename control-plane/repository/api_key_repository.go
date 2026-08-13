package repository

import (
	"context"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionApiKey = "api_keys"

type ApiKeyRepository struct {
	db *mongo.Database
}

func NewApiKeyRepository(db *mongo.Database) *ApiKeyRepository {
	return &ApiKeyRepository{db: db}
}

var apiKeySearchFields = []string{"name"}

var apiKeyIgnoredParams = append([]string{"ownerBy"}, tool.CommonIgnoredParams...)

func (r *ApiKeyRepository) buildFilter(params url.Values, ownerIDs []bson.ObjectID, excludeOwnerIDs []bson.ObjectID) (bson.M, error) {
	filter := bson.M{"deletedAt": bson.M{opExists: false}}

	switch len(ownerIDs) {
	case 1:
		filter["ownerBy"] = ownerIDs[0]
	default:
		if len(ownerIDs) > 1 {
			filter["ownerBy"] = bson.M{"$in": ownerIDs}
		}
	}

	if len(excludeOwnerIDs) == 1 {
		existing, ok := filter["ownerBy"]
		if ok {
			filter["ownerBy"] = bson.M{"$eq": existing, "$ne": excludeOwnerIDs[0]}
		} else {
			filter["ownerBy"] = bson.M{"$ne": excludeOwnerIDs[0]}
		}
	} else if len(excludeOwnerIDs) > 1 {
		if _, ok := filter["ownerBy"]; ok {
			filter["ownerBy"].(bson.M)["$nin"] = excludeOwnerIDs
		} else {
			filter["ownerBy"] = bson.M{"$nin": excludeOwnerIDs}
		}
	}

	if len(params) > 0 {
		conditions, err := tool.GenerateFilterBson(params, apiKeyIgnoredParams)
		if err != nil {
			return nil, err
		}
		if len(conditions) > 0 {
			filter["$and"] = conditions
		}
	}

	if s := params.Get("search"); s != "" {
		s = tool.EscapeRegexPattern(s)
		or := make([]bson.M, len(apiKeySearchFields))
		for i, f := range apiKeySearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}

	return filter, nil
}

var userLookupProject = bson.M{
	"firstName":   1,
	"lastName":    1,
	"displayName": 1,
	"email":       1,
	"username":    1,
	"avatar":      1,
	"role":        1,
}

var userLookupStages = mongo.Pipeline{
	{{Key: "$lookup", Value: bson.M{
		"from":         "users",
		"localField":   "ownerBy",
		"foreignField": "_id",
		"as":           "_ownerUser",
		"pipeline":     mongo.Pipeline{{{Key: "$project", Value: userLookupProject}}},
	}}},
	{{Key: "$lookup", Value: bson.M{
		"from":         "users",
		"localField":   "createdBy",
		"foreignField": "_id",
		"as":           "_createdUser",
		"pipeline":     mongo.Pipeline{{{Key: "$project", Value: userLookupProject}}},
	}}},
	{{Key: "$set", Value: bson.M{
		"ownerByUser":   bson.M{"$arrayElemAt": bson.A{"$_ownerUser", 0}},
		"createdByUser": bson.M{"$arrayElemAt": bson.A{"$_createdUser", 0}},
	}}},
	{{Key: "$unset", Value: bson.A{"_ownerUser", "_createdUser"}}},
}

func (r *ApiKeyRepository) FindAll(ctx context.Context, params url.Values, ownerIDs []bson.ObjectID, excludeOwnerIDs []bson.ObjectID) ([]*model.ApiKey, int64, error) {
	filter, err := r.buildFilter(params, ownerIDs, excludeOwnerIDs)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionApiKey).CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	limit := tool.Limit
	if v := params.Get("limit"); v != "" {
		if n, e := tool.ParseLimit(v); e == nil {
			limit = n
		}
	}

	offset := tool.Offset
	if v := params.Get("offset"); v != "" {
		if n, e := tool.ParseOffset(v); e == nil {
			offset = n
		}
	}

	pipeline := mongo.Pipeline{{{Key: "$match", Value: filter}}}

	if sort := params.Get("sort"); sort != "" {
		if sortBson, e := tool.OptionSortBson(sort); e == nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortBson}})
		}
	}

	pipeline = append(pipeline,
		bson.D{{Key: "$skip", Value: int64(offset)}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	)
	pipeline = append(pipeline, userLookupStages...)

	cursor, err := r.db.Collection(collectionApiKey).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.ApiKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *ApiKeyRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.ApiKey, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}

	pipeline := mongo.Pipeline{{{Key: "$match", Value: filter}}}
	pipeline = append(pipeline, userLookupStages...)

	cursor, err := r.db.Collection(collectionApiKey).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []model.ApiKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return &results[0], nil
}

// FindByIDs คืน api-key หลายตัวใน 1 query (ผ่าน $in) แทนการเรียก FindByID ทีละตัวใน loop —
// ไม่ join owner (userLookupStages) เพราะ use case นี้ (resolve ชื่อ) ต้องการแค่ field Name
func (r *ApiKeyRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.ApiKey, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{
		"_id":       bson.M{"$in": ids},
		"deletedAt": bson.M{opExists: false},
	}
	cursor, err := r.db.Collection(collectionApiKey).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.ApiKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *ApiKeyRepository) FindByKey(ctx context.Context, key string) (*model.ApiKey, error) {
	filter := bson.M{
		"apiKey":    key,
		"enabled":   true,
		"deletedAt": bson.M{opExists: false},
	}

	var result model.ApiKey
	err := r.db.Collection(collectionApiKey).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ApiKeyRepository) FindAllActive(ctx context.Context) ([]*model.ApiKey, error) {
	cursor, err := r.db.Collection(collectionApiKey).Find(ctx, bson.M{
		"enabled":   true,
		"deletedAt": bson.M{opExists: false},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*model.ApiKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// FindExpiring คืน api key ที่ยัง active และ expiredAt อยู่ในช่วง (from, to] — แบ่งหน้าด้วย limit/offset
// enabled: $ne false = true หรือ default (nil) ถือว่า active; sort _id เพื่อ pagination เสถียร
func (r *ApiKeyRepository) FindExpiring(ctx context.Context, from, to time.Time, limit, offset int) ([]*model.ApiKey, error) {
	filter := bson.M{
		"enabled":   bson.M{"$ne": false},
		"deletedAt": bson.M{opExists: false},
		"expiredAt": bson.M{"$gt": from, "$lte": to},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))
	cursor, err := r.db.Collection(collectionApiKey).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*model.ApiKey
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *ApiKeyRepository) Insert(ctx context.Context, doc *model.ApiKey) error {
	_, err := r.db.Collection(collectionApiKey).InsertOne(ctx, doc)
	return err
}

func (r *ApiKeyRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionApiKey).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *ApiKeyRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiKey).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *ApiKeyRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiKey).DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ApiKeyRepository) BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiKey).UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": ids}, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *ApiKeyRepository) BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiKey).DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}
