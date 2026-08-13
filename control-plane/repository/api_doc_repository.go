package repository

import (
	"context"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"net/url"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionApiDoc = "api_docs"

type ApiDocRepository struct {
	db *mongo.Database
}

func NewApiDocRepository(db *mongo.Database) *ApiDocRepository {
	return &ApiDocRepository{db: db}
}

var apiDocSearchFields = []string{"info.title", "paths.path"}
var pathSearchFields = []string{"path", "operationId"}

func (r *ApiDocRepository) buildFilter(params url.Values, searchFields []string) (bson.M, error) {
	filter := bson.M{
		"deletedAt": bson.M{opExists: false},
	}

	if len(params) > 0 {
		conditions, err := tool.GenerateFilterBson(params, tool.CommonIgnoredParams)
		if err != nil {
			return nil, err
		}
		if len(conditions) > 0 {
			filter["$and"] = conditions
		}
	}

	if s := params.Get("search"); s != "" {
		s = tool.EscapeRegexPattern(s)
		or := make([]bson.M, len(searchFields))
		for i, f := range searchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *ApiDocRepository) FindAll(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error) {
	filter, err := r.buildFilter(params, apiDocSearchFields)
	if err != nil {
		return nil, 0, err
	}
	limit, offset := r.parseLimitOffset(params)

	total, err := r.db.Collection(collectionApiDoc).CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	findOpts := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	if sort := params.Get("sort"); sort != "" {
		if sortBson, e := tool.OptionSortBson(sort); e == nil {
			findOpts.SetSort(sortBson)
		}
	}

	findOpts.SetProjection(bson.M{"paths": 0})

	cursor, err := r.db.Collection(collectionApiDoc).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.ApiDoc
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

func (r *ApiDocRepository) FindByID(ctx context.Context, id bson.ObjectID, params *url.Values) (*model.ApiDoc, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}

	if params != nil && len(*params) > 0 {
		conditions, err := tool.GenerateFilterBson(*params, tool.CommonIgnoredParams)
		if err != nil {
			return nil, err
		}
		if len(conditions) > 0 {
			filter["$and"] = conditions
		}
	}

	var result model.ApiDoc
	err := r.db.Collection(collectionApiDoc).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *ApiDocRepository) Insert(ctx context.Context, doc *model.ApiDoc) error {
	_, err := r.db.Collection(collectionApiDoc).InsertOne(ctx, doc)
	return err
}

func (r *ApiDocRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionApiDoc).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *ApiDocRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiDoc).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *ApiDocRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionApiDoc).DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *ApiDocRepository) FindPathsByID(ctx context.Context, id bson.ObjectID, apiDocParams, pathParams url.Values) ([]*model.ApiDocPath, int64, error) {
	apiDocFilter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}

	if len(apiDocParams) > 0 {
		conditions, err := tool.GenerateFilterBson(apiDocParams, tool.CommonIgnoredParams)
		if err != nil {
			return nil, 0, err
		}
		if len(conditions) > 0 {
			apiDocFilter["$and"] = conditions
		}
	}

	// for check is the doc exsited because pipeline return empty as a [] not error no doc
	count, err := r.db.Collection(collectionApiDoc).CountDocuments(ctx, apiDocFilter)
	if err != nil {
		return nil, 0, err
	}
	if count == 0 {
		return nil, 0, mongo.ErrNoDocuments
	}

	limit, offset := r.parseLimitOffset(pathParams)

	pathFilter, err := r.buildFilter(pathParams, pathSearchFields)
	if err != nil {
		return nil, 0, err
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: apiDocFilter}},
		bson.D{{Key: "$unwind", Value: "$paths"}},
		bson.D{{Key: "$replaceRoot", Value: bson.M{"newRoot": "$paths"}}},
		bson.D{{Key: "$match", Value: pathFilter}},
	}

	if sort := pathParams.Get("sort"); sort != "" {
		if sortBson, e := tool.OptionSortBson(sort); e == nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortBson}})
		}
	}

	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: bson.D{
		{Key: "total", Value: bson.A{bson.D{{Key: "$count", Value: "n"}}}},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "$skip", Value: offset}},
			bson.D{{Key: "$limit", Value: limit}},
		}},
	}}})

	cursor, err := r.db.Collection(collectionApiDoc).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var facetResult struct {
		Total []struct {
			N int64 `bson:"n"`
		} `bson:"total"`
		Items []*model.ApiDocPath `bson:"items"`
	}
	if !cursor.Next(ctx) {
		return nil, 0, mongo.ErrNoDocuments
	}
	if err := cursor.Decode(&facetResult); err != nil {
		return nil, 0, err
	}

	var total int64
	if len(facetResult.Total) > 0 {
		total = facetResult.Total[0].N
	}
	items := facetResult.Items
	if items == nil {
		items = []*model.ApiDocPath{}
	}

	return items, total, nil
}

func (r *ApiDocRepository) parseLimitOffset(params url.Values) (int, int) {
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
	return limit, offset
}
