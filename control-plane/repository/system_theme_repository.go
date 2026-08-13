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

const collectionSystemTheme = "system_themes"

type SystemThemeRepository struct {
	db *mongo.Database
}

func NewSystemThemeRepository(db *mongo.Database) *SystemThemeRepository {
	return &SystemThemeRepository{db: db}
}

var systemThemeSearchFields = []string{"name", "description"}

func (r *SystemThemeRepository) buildFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(systemThemeSearchFields))
		for i, f := range systemThemeSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *SystemThemeRepository) FindAll(ctx context.Context, params url.Values) ([]*model.SystemTheme, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionSystemTheme).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionSystemTheme).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.SystemTheme
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *SystemThemeRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.SystemTheme, error) {
	filter := bson.M{"_id": id, "deletedAt": bson.M{opExists: false}}
	var result model.SystemTheme
	if err := r.db.Collection(collectionSystemTheme).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *SystemThemeRepository) FindDefault(ctx context.Context) (*model.SystemTheme, error) {
	filter := bson.M{"default": true, "deletedAt": bson.M{opExists: false}}
	var result model.SystemTheme
	if err := r.db.Collection(collectionSystemTheme).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *SystemThemeRepository) Insert(ctx context.Context, doc *model.SystemTheme) error {
	_, err := r.db.Collection(collectionSystemTheme).InsertOne(ctx, doc)
	return err
}

func (r *SystemThemeRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionSystemTheme).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

// UnsetDefault ล้าง default=true จาก theme ทั้งหมด (ก่อน set default ใหม่)
func (r *SystemThemeRepository) UnsetDefault(ctx context.Context) error {
	_, err := r.db.Collection(collectionSystemTheme).UpdateMany(
		ctx,
		bson.M{"default": true, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"default": false}},
	)
	return err
}

// SoftDelete ตั้ง deletedAt/deletedBy แต่ไม่ลบออกจาก DB
func (r *SystemThemeRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionSystemTheme).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}
