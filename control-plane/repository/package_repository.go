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

const collectionPackage = "packages"

type PackageRepository struct {
	db *mongo.Database
}

func NewPackageRepository(db *mongo.Database) *PackageRepository {
	return &PackageRepository{db: db}
}

var packageSearchFields = []string{"name", "alias", "description"}

func (r *PackageRepository) buildFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(packageSearchFields))
		for i, f := range packageSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *PackageRepository) FindAll(ctx context.Context, params url.Values) ([]*model.Package, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionPackage).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionPackage).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.Package
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *PackageRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.Package, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.Package
	if err := r.db.Collection(collectionPackage).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FindByIDs คืน package หลายตัวใน 1 query (ผ่าน $in) แทนการเรียก FindByID ทีละตัวใน loop
func (r *PackageRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.Package, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{
		"_id":       bson.M{"$in": ids},
		"deletedAt": bson.M{opExists: false},
	}
	cursor, err := r.db.Collection(collectionPackage).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.Package
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *PackageRepository) FindByAlias(ctx context.Context, alias string) (*model.Package, error) {
	filter := bson.M{
		"alias":     alias,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.Package
	if err := r.db.Collection(collectionPackage).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *PackageRepository) Insert(ctx context.Context, doc *model.Package) error {
	_, err := r.db.Collection(collectionPackage).InsertOne(ctx, doc)
	return err
}

func (r *PackageRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionPackage).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *PackageRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionPackage).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *PackageRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionPackage).DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *PackageRepository) IncrUserCount(ctx context.Context, id bson.ObjectID, delta int64) error {
	_, err := r.db.Collection(collectionPackage).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$inc": bson.M{"userCount": delta}},
	)
	return err
}

func (r *PackageRepository) IncrServiceCount(ctx context.Context, id bson.ObjectID, delta int64) error {
	_, err := r.db.Collection(collectionPackage).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$inc": bson.M{"serviceCount": delta}},
	)
	return err
}

// SyncServiceCounts อัปเดต serviceCount ของทุก package ให้ตรงกับจำนวน service จริงใน DB
// ใช้เรียกตอน startup เพื่อแก้ค่าที่อาจผิดพลาดก่อนหน้า
func (r *PackageRepository) SyncServiceCounts(ctx context.Context) error {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": "$packageId", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.db.Collection(collectionPackageSvcLink).Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var row struct {
			ID    bson.ObjectID `bson:"_id"`
			Count int64         `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}
		_, _ = r.db.Collection(collectionPackage).UpdateOne(
			ctx,
			bson.M{"_id": row.ID},
			bson.M{"$set": bson.M{"serviceCount": row.Count}},
		)
	}

	// zero out packages with no services
	_, err = r.db.Collection(collectionPackage).UpdateMany(
		ctx,
		bson.M{"deletedAt": bson.M{opExists: false}, "serviceCount": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"serviceCount": int64(0)}},
	)
	return err
}

// SyncUserCounts อัปเดต userCount ของทุก package ให้ตรงกับจำนวน user จริงใน DB
// ใช้เรียกตอน startup เพื่อแก้ค่าที่อาจผิดพลาดก่อนหน้า
func (r *PackageRepository) SyncUserCounts(ctx context.Context) error {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"packageId": bson.M{"$exists": true}, "deletedAt": bson.M{opExists: false}}}},
		{{Key: "$group", Value: bson.M{"_id": "$packageId", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := r.db.Collection("users").Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var row struct {
			ID    bson.ObjectID `bson:"_id"`
			Count int64         `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			continue
		}
		_, _ = r.db.Collection(collectionPackage).UpdateOne(
			ctx,
			bson.M{"_id": row.ID},
			bson.M{"$set": bson.M{"userCount": row.Count}},
		)
	}

	// zero out packages with no users
	_, err = r.db.Collection(collectionPackage).UpdateMany(
		ctx,
		bson.M{"deletedAt": bson.M{opExists: false}, "userCount": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"userCount": int64(0)}},
	)
	return err
}
