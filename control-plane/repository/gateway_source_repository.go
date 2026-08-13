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

const collectionGatewaySource = "gateway_sources"

type GatewaySourceRepository struct {
	db *mongo.Database
}

func NewGatewaySourceRepository(db *mongo.Database) *GatewaySourceRepository {
	return &GatewaySourceRepository{db: db}
}

var gatewaySourceSearchFields = []string{"alias", "name", "description"}

func (r *GatewaySourceRepository) buildFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(gatewaySourceSearchFields))
		for i, f := range gatewaySourceSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *GatewaySourceRepository) FindAll(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionGatewaySource).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionGatewaySource).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.GatewaySource
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *GatewaySourceRepository) FindAllActive(ctx context.Context) ([]*model.GatewaySource, error) {
	filter := bson.M{"deletedAt": bson.M{opExists: false}}
	cursor, err := r.db.Collection(collectionGatewaySource).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*model.GatewaySource
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *GatewaySourceRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewaySource, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.GatewaySource
	if err := r.db.Collection(collectionGatewaySource).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GatewaySourceRepository) FindByAlias(ctx context.Context, alias string) (*model.GatewaySource, error) {
	filter := bson.M{
		"alias":     alias,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.GatewaySource
	if err := r.db.Collection(collectionGatewaySource).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GatewaySourceRepository) FindByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error) {
	if id, err := bson.ObjectIDFromHex(idOrAlias); err == nil {
		return r.FindByID(ctx, id)
	}
	return r.FindByAlias(ctx, idOrAlias)
}

func (r *GatewaySourceRepository) Insert(ctx context.Context, doc *model.GatewaySource) error {
	_, err := r.db.Collection(collectionGatewaySource).InsertOne(ctx, doc)
	return err
}

func (r *GatewaySourceRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionGatewaySource).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *GatewaySourceRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionGatewaySource).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *GatewaySourceRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionGatewaySource).DeleteOne(ctx, bson.M{"_id": id})
	return err
}
