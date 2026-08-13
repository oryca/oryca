package repository

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	collectionGatewayService = "gateway_services"
	stageMatch               = "$match"
)

type GatewayServiceRepository struct {
	db *mongo.Database
}

func NewGatewayServiceRepository(db *mongo.Database) *GatewayServiceRepository {
	return &GatewayServiceRepository{db: db}
}

var gatewayServiceSearchFields = []string{"name", "description", "basePath"}

func (r *GatewayServiceRepository) buildFilter(params url.Values) (bson.M, error) {
	filter := bson.M{"deletedAt": bson.M{opExists: false}}

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
		or := make([]bson.M, len(gatewayServiceSearchFields))
		for i, f := range gatewayServiceSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

// joinedSourceFields คือ prefix ของ query params ที่ filter บน joined source field
// ต้องใส่ใน $match หลัง $lookup+$set เท่านั้น
const joinedSourcePrefix = "resourcePaths.source."

// splitJoinedParams แยก params ที่ต้องกรองหลัง $lookup ออกจาก params ปกติ
func splitJoinedParams(params url.Values) (pre url.Values, post url.Values) {
	pre = make(url.Values)
	post = make(url.Values)
	for k, vs := range params {
		if strings.HasPrefix(k, joinedSourcePrefix) {
			post[k] = vs
		} else {
			pre[k] = vs
		}
	}
	return
}

func buildEmbedStages() []bson.D {
	return []bson.D{
		{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "gateway_sources"},
			{Key: "localField", Value: "resourcePaths.sourceAlias"},
			{Key: "foreignField", Value: "alias"},
			{Key: "as", Value: "_sources"},
		}}},
		{{Key: "$set", Value: bson.M{
			"resourcePaths": bson.M{
				"$map": bson.M{
					"input": "$resourcePaths",
					"as":    "rp",
					"in": bson.M{
						"$mergeObjects": bson.A{
							"$$rp",
							bson.M{
								"source": bson.M{
									"$first": bson.M{
										"$filter": bson.M{
											"input": "$_sources",
											"cond": bson.M{"$and": bson.A{
												bson.M{"$eq": bson.A{"$$this.alias", "$$rp.sourceAlias"}},
												bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$$this.deletedAt", nil}}, nil}},
											}},
										},
									},
								},
							},
						},
					},
				},
			},
		}}},
		{{Key: "$unset", Value: "_sources"}},
	}
}

func (r *GatewayServiceRepository) FindAll(ctx context.Context, params url.Values) ([]*model.GatewayService, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionGatewayService).CountDocuments(ctx, filter)
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

	findOpts := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	if sort := params.Get("sort"); sort != "" {
		if sortBson, e := tool.OptionSortBson(sort); e == nil {
			findOpts.SetSort(sortBson)
		}
	}

	cursor, err := r.db.Collection(collectionGatewayService).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.GatewayService
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func parseLimitOffset(params url.Values) (limit, offset int) {
	limit = tool.Limit
	if v := params.Get("limit"); v != "" {
		if n, e := tool.ParseLimit(v); e == nil {
			limit = n
		}
	}
	offset = tool.Offset
	if v := params.Get("offset"); v != "" {
		if n, e := tool.ParseOffset(v); e == nil {
			offset = n
		}
	}
	return
}

func buildPostMatchStage(postParams url.Values) (bson.D, error) {
	if len(postParams) == 0 {
		return nil, nil
	}
	postFilter, err := tool.GenerateFilterBson(postParams, nil)
	if err != nil {
		return nil, err
	}
	if len(postFilter) == 0 {
		return nil, nil
	}
	return bson.D{{Key: stageMatch, Value: bson.M{"$and": postFilter}}}, nil
}

func decodeFacetResult(facetResult []struct {
	Total []struct {
		N int64 `bson:"n"`
	} `bson:"total"`
	Items []*model.GatewayServiceWithSource `bson:"items"`
}) ([]*model.GatewayServiceWithSource, int64) {
	if len(facetResult) == 0 {
		return []*model.GatewayServiceWithSource{}, 0
	}
	var total int64
	if len(facetResult[0].Total) > 0 {
		total = facetResult[0].Total[0].N
	}
	items := facetResult[0].Items
	if items == nil {
		items = []*model.GatewayServiceWithSource{}
	}
	return items, total
}

func (r *GatewayServiceRepository) FindAllWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error) {
	preParams, postParams := splitJoinedParams(params)

	preFilter, err := r.buildFilter(preParams)
	if err != nil {
		return nil, 0, err
	}

	limit, offset := parseLimitOffset(params)

	pipeline := mongo.Pipeline{bson.D{{Key: stageMatch, Value: preFilter}}}
	pipeline = append(pipeline, buildEmbedStages()...)

	if postStage, err := buildPostMatchStage(postParams); err != nil {
		return nil, 0, err
	} else if postStage != nil {
		pipeline = append(pipeline, postStage)
	}

	if sortVal := params.Get("sort"); sortVal != "" {
		if sortBson, e := tool.OptionSortBson(sortVal); e == nil {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: sortBson}})
		}
	}

	pipeline = append(pipeline, bson.D{{Key: "$facet", Value: bson.D{
		{Key: "total", Value: bson.A{bson.D{{Key: "$count", Value: "n"}}}},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "$skip", Value: int64(offset)}},
			bson.D{{Key: "$limit", Value: int64(limit)}},
		}},
	}}})

	cursor, err := r.db.Collection(collectionGatewayService).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var facetResult []struct {
		Total []struct {
			N int64 `bson:"n"`
		} `bson:"total"`
		Items []*model.GatewayServiceWithSource `bson:"items"`
	}
	if err := cursor.All(ctx, &facetResult); err != nil {
		return nil, 0, err
	}

	items, total := decodeFacetResult(facetResult)
	return items, total, nil
}

func (r *GatewayServiceRepository) FindByIDWithSource(ctx context.Context, id bson.ObjectID) (*model.GatewayServiceWithSource, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}

	pipeline := mongo.Pipeline{bson.D{{Key: stageMatch, Value: filter}}}
	pipeline = append(pipeline, buildEmbedStages()...)
	pipeline = append(pipeline, bson.D{{Key: "$limit", Value: int64(1)}})

	cursor, err := r.db.Collection(collectionGatewayService).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.GatewayServiceWithSource
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return results[0], nil
}

func (r *GatewayServiceRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.GatewayService
	if err := r.db.Collection(collectionGatewayService).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FindByIDs คืน service หลายตัวใน 1 query (ผ่าน $in) แทนการเรียก FindByID ทีละตัวใน loop
func (r *GatewayServiceRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.GatewayService, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{
		"_id":       bson.M{"$in": ids},
		"deletedAt": bson.M{opExists: false},
	}
	cursor, err := r.db.Collection(collectionGatewayService).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.GatewayService
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *GatewayServiceRepository) FindAllActive(ctx context.Context) ([]*model.GatewayService, error) {
	filter := bson.M{"deletedAt": bson.M{opExists: false}}
	cursor, err := r.db.Collection(collectionGatewayService).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*model.GatewayService
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *GatewayServiceRepository) FindByBasePath(ctx context.Context, basePath string) (*model.GatewayService, error) {
	filter := bson.M{
		"basePath":  basePath,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.GatewayService
	if err := r.db.Collection(collectionGatewayService).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GatewayServiceRepository) CountBySourceAlias(ctx context.Context, sourceAlias string) (int64, error) {
	filter := bson.M{
		"resourcePaths.sourceAlias": sourceAlias,
		"deletedAt":                 bson.M{opExists: false},
	}
	return r.db.Collection(collectionGatewayService).CountDocuments(ctx, filter)
}

func (r *GatewayServiceRepository) Insert(ctx context.Context, doc *model.GatewayService) error {
	_, err := r.db.Collection(collectionGatewayService).InsertOne(ctx, doc)
	return err
}

func (r *GatewayServiceRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionGatewayService).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *GatewayServiceRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionGatewayService).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *GatewayServiceRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionGatewayService).DeleteOne(ctx, bson.M{"_id": id})
	return err
}
