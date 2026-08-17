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

const collectionUser = "users"

type UserRepository struct {
	db *mongo.Database
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{db: db}
}

var userSearchFields = []string{"firstName", "lastName", "displayName", "email", "username", "organization", "phone"}

func (r *UserRepository) buildUserFilter(params url.Values, excludeRoot bool) (bson.M, error) {
	filter := bson.M{"deletedAt": bson.M{opExists: false}}
	if excludeRoot {
		filter["role"] = bson.M{"$ne": "root"}
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
		or := make([]bson.M, len(userSearchFields))
		for i, f := range userSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *UserRepository) FindAll(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error) {
	filter, err := r.buildUserFilter(params, excludeRoot)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionUser).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionUser).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.User
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error) {
	filter := bson.M{
		"_id":       id,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.User
	if err := r.db.Collection(collectionUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FindByRoles คืน user ทั้งหมดที่ role อยู่ใน roles ที่กำหนด. ใช้ fan-out แจ้งเตือนถึง admin
func (r *UserRepository) FindByRoles(ctx context.Context, roles []string) ([]*model.User, error) {
	filter := bson.M{"role": bson.M{"$in": roles}, "deletedAt": bson.M{opExists: false}}
	cursor, err := r.db.Collection(collectionUser).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []*model.User
	return results, cursor.All(ctx, &results)
}

// FindByIDs คืน user หลายคนใน 1 query (ผ่าน $in) แทนการเรียก FindByID ทีละคนใน loop —
// ใช้ตอนต้องการ owner ของ record จำนวนมาก (เช่น api-key sync) เพื่อลด round-trip จาก O(n) เหลือ O(1)
func (r *UserRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	filter := bson.M{
		"_id":       bson.M{"$in": ids},
		"deletedAt": bson.M{opExists: false},
	}
	cursor, err := r.db.Collection(collectionUser).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.User
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	filter := bson.M{
		"email":     email,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.User
	if err := r.db.Collection(collectionUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *UserRepository) FindByPhone(ctx context.Context, phone string) (*model.User, error) {
	filter := bson.M{
		"phone":     phone,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.User
	if err := r.db.Collection(collectionUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *UserRepository) FindByUsernameExact(ctx context.Context, username string) (*model.User, error) {
	filter := bson.M{
		"username":  username,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.User
	if err := r.db.Collection(collectionUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FindByUsername ค้นหา user ด้วย email, phone หรือ username (สำหรับ login)
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	filter := bson.M{
		"$or": bson.A{
			bson.M{"email": username},
			bson.M{"phone": username},
			bson.M{"username": username},
		},
		"deletedAt": bson.M{opExists: false},
	}
	var result model.User
	if err := r.db.Collection(collectionUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *UserRepository) Insert(ctx context.Context, doc *model.User) error {
	_, err := r.db.Collection(collectionUser).InsertOne(ctx, doc)
	return err
}

func (r *UserRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionUser).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

func (r *UserRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionUser).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *UserRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionUser).DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *UserRepository) BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionUser).UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": ids}, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *UserRepository) BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error {
	_, err := r.db.Collection(collectionUser).DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	return err
}

func (r *UserRepository) CountByPackageID(ctx context.Context, packageID bson.ObjectID) (int64, error) {
	return r.db.Collection(collectionUser).CountDocuments(ctx, bson.M{
		"packageId": packageID,
		"deletedAt": bson.M{opExists: false},
	})
}

func (r *UserRepository) CountByPackageIDs(ctx context.Context, ids []bson.ObjectID) (map[bson.ObjectID]int64, error) {
	pipeline := bson.A{
		bson.M{"$match": bson.M{"packageId": bson.M{"$in": ids}, "deletedAt": bson.M{opExists: false}}},
		bson.M{"$group": bson.M{"_id": "$packageId", "count": bson.M{"$sum": 1}}},
	}
	cursor, err := r.db.Collection(collectionUser).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	counts := map[bson.ObjectID]int64{}
	for cursor.Next(ctx) {
		var row struct {
			ID    bson.ObjectID `bson:"_id"`
			Count int64         `bson:"count"`
		}
		if err := cursor.Decode(&row); err == nil {
			counts[row.ID] = row.Count
		}
	}
	return counts, nil
}

func (r *UserRepository) FindByPackageID(ctx context.Context, packageID bson.ObjectID, params url.Values) ([]*model.User, int64, error) {
	filter, err := r.buildUserFilter(params, false)
	if err != nil {
		return nil, 0, err
	}
	filter["packageId"] = packageID

	total, err := r.db.Collection(collectionUser).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionUser).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.User
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// CountByOldPackageIDs returns a map of old packageId → number of users in userIDs
// that currently belong to that package and will be moved to a different package.
func (r *UserRepository) CountByOldPackageIDs(ctx context.Context, userIDs []bson.ObjectID, newPackageID bson.ObjectID) (map[bson.ObjectID]int64, error) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.M{
			"_id":       bson.M{"$in": userIDs},
			"packageId": bson.M{"$exists": true, "$ne": newPackageID},
			"deletedAt": bson.M{opExists: false},
		}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":   "$packageId",
			"count": bson.M{"$sum": 1},
		}}},
	}
	cursor, err := r.db.Collection(collectionUser).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[bson.ObjectID]int64)
	for cursor.Next(ctx) {
		var row struct {
			ID    bson.ObjectID `bson:"_id"`
			Count int64         `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		result[row.ID] = row.Count
	}
	return result, nil
}

func (r *UserRepository) BulkSetPackageID(ctx context.Context, userIDs []bson.ObjectID, packageID bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error) {
	result, err := r.db.Collection(collectionUser).UpdateMany(
		ctx,
		bson.M{
			"_id":       bson.M{"$in": userIDs},
			"packageId": bson.M{"$ne": packageID},
			"deletedAt": bson.M{opExists: false},
		},
		bson.M{"$set": bson.M{"packageId": packageID, "updatedAt": now, "updatedBy": updatedBy}},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

func (r *UserRepository) BulkUnsetPackageID(ctx context.Context, packageID bson.ObjectID, userIDs []bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error) {
	result, err := r.db.Collection(collectionUser).UpdateMany(
		ctx,
		bson.M{"_id": bson.M{"$in": userIDs}, "packageId": packageID, "deletedAt": bson.M{opExists: false}},
		bson.M{"$unset": bson.M{"packageId": ""}, "$set": bson.M{"updatedAt": now, "updatedBy": updatedBy}},
	)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}
