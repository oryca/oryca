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

const collectionGroup = "groups"

type GroupRepository struct {
	db *mongo.Database
}

func NewGroupRepository(db *mongo.Database) *GroupRepository {
	return &GroupRepository{db: db}
}

var groupSearchFields = []string{"name", "alias", "description"}

func (r *GroupRepository) buildGroupFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(groupSearchFields))
		for i, f := range groupSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *GroupRepository) FindAll(ctx context.Context, params url.Values) ([]*model.Group, int64, error) {
	filter, err := r.buildGroupFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionGroup).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionGroup).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.Group
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *GroupRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.Group, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.Group
	if err := r.db.Collection(collectionGroup).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GroupRepository) FindByAlias(ctx context.Context, alias string) (*model.Group, error) {
	filter := bson.M{
		"alias":     alias,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.Group
	if err := r.db.Collection(collectionGroup).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GroupRepository) Insert(ctx context.Context, doc *model.Group) error {
	_, err := r.db.Collection(collectionGroup).InsertOne(ctx, doc)
	return err
}

func (r *GroupRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionGroup).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *GroupRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroup).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *GroupRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroup).DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// FindDescendantGroupIDs returns the given groupID plus all descendant group IDs recursively.
func (r *GroupRepository) FindDescendantGroupIDs(ctx context.Context, groupID bson.ObjectID) ([]bson.ObjectID, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{"_id": groupID}}},
		bson.D{{Key: "$graphLookup", Value: bson.D{
			{Key: "from", Value: collectionGroup},
			{Key: "startWith", Value: "$_id"},
			{Key: "connectFromField", Value: "_id"},
			{Key: "connectToField", Value: "parentId"},
			{Key: "as", Value: "_descendants"},
			{Key: "restrictSearchWithMatch", Value: bson.M{"deletedAt": bson.M{opExists: false}}},
		}}},
	}

	cursor, err := r.db.Collection(collectionGroup).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result struct {
		ID          bson.ObjectID `bson:"_id"`
		Descendants []struct {
			ID bson.ObjectID `bson:"_id"`
		} `bson:"_descendants"`
	}
	if !cursor.Next(ctx) {
		return nil, mongo.ErrNoDocuments
	}
	if err := cursor.Decode(&result); err != nil {
		return nil, err
	}

	ids := make([]bson.ObjectID, 0, len(result.Descendants)+1)
	ids = append(ids, result.ID)
	for _, d := range result.Descendants {
		ids = append(ids, d.ID)
	}
	return ids, nil
}
