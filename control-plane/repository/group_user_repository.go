package repository

import (
	"context"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const collectionGroupUser = "group_users"

type GroupUserRepository struct {
	db *mongo.Database
}

func NewGroupUserRepository(db *mongo.Database) *GroupUserRepository {
	return &GroupUserRepository{db: db}
}

func (r *GroupUserRepository) buildGroupUserPipeline(matchFilter bson.M, params url.Values) mongo.Pipeline {
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

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: matchFilter}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "users"},
			{Key: "localField", Value: "user._id"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "_userInfo"},
		}}},
		bson.D{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$_userInfo"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "groups"},
			{Key: "localField", Value: "group._id"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "_groupInfo"},
		}}},
		bson.D{{Key: "$unwind", Value: bson.D{
			{Key: "path", Value: "$_groupInfo"},
			{Key: "preserveNullAndEmptyArrays", Value: true},
		}}},
		bson.D{{Key: "$set", Value: bson.M{
			"user.accountType":  "$_userInfo.accountType",
			"user.email":        "$_userInfo.email",
			"user.phone":        "$_userInfo.phone",
			"user.countryCode":  "$_userInfo.countryCode",
			"user.username":     "$_userInfo.username",
			"user.firstName":    "$_userInfo.firstName",
			"user.lastName":     "$_userInfo.lastName",
			"user.displayName":  "$_userInfo.displayName",
			"user.organization": "$_userInfo.organization",
			"user.avatar":       "$_userInfo.avatar",
			"user.role":         "$_userInfo.role",
			"user.verified":     "$_userInfo.verified",
			"user.enabled":      "$_userInfo.enabled",
			"user.lastLogin":    "$_userInfo.lastLogin",
			"user.expiredAt":    "$_userInfo.expiredAt",
			"user.properties":   "$_userInfo.properties",
			"group.name":        "$_groupInfo.name",
			"group.alias":       "$_groupInfo.alias",
		}}},
		bson.D{{Key: "$unset", Value: bson.A{"_userInfo", "_groupInfo"}}},
		bson.D{{Key: "$skip", Value: int64(offset)}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	}

	return pipeline
}

func (r *GroupUserRepository) FindByGroup(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error) {
	filter := bson.M{
		"group._id": groupID,
		"deletedAt": bson.M{opExists: false},
	}

	total, err := r.db.Collection(collectionGroupUser).CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	pipeline := r.buildGroupUserPipeline(filter, params)
	cursor, err := r.db.Collection(collectionGroupUser).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.GroupUserDetail
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *GroupUserRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.GroupUserDetail, error) {
	filter := bson.M{
		"_id":       bson.M{"$in": ids},
		"deletedAt": bson.M{opExists: false},
	}

	pipeline := r.buildGroupUserPipeline(filter, url.Values{
		"limit": []string{"1000"},
	})
	cursor, err := r.db.Collection(collectionGroupUser).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.GroupUserDetail
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *GroupUserRepository) FindByUserAndGroup(ctx context.Context, userID, groupID bson.ObjectID) (*model.GroupUser, error) {
	filter := bson.M{
		"user._id":  userID,
		"group._id": groupID,
		"deletedAt": bson.M{opExists: false},
	}
	var result model.GroupUser
	if err := r.db.Collection(collectionGroupUser).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GroupUserRepository) CountByGroup(ctx context.Context, groupID bson.ObjectID) (int, error) {
	filter := bson.M{
		"group._id": groupID,
		"deletedAt": bson.M{opExists: false},
	}
	count, err := r.db.Collection(collectionGroupUser).CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *GroupUserRepository) FindUserIDsByGroupIDs(ctx context.Context, groupIDs []bson.ObjectID) ([]bson.ObjectID, error) {
	filter := bson.M{
		"group._id": bson.M{"$in": groupIDs},
		"deletedAt": bson.M{opExists: false},
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$user._id"}}},
	}
	cursor, err := r.db.Collection(collectionGroupUser).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		ID bson.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	ids := make([]bson.ObjectID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

func (r *GroupUserRepository) InsertMany(ctx context.Context, docs []interface{}) error {
	_, err := r.db.Collection(collectionGroupUser).InsertMany(ctx, docs)
	return err
}

func (r *GroupUserRepository) SoftDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroupUser).UpdateMany(
		ctx,
		bson.M{
			"group._id": groupID,
			"user._id":  bson.M{"$in": userIDs},
			"deletedAt": bson.M{opExists: false},
		},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *GroupUserRepository) HardDeleteByGroupAndUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroupUser).DeleteMany(
		ctx,
		bson.M{
			"group._id": groupID,
			"user._id":  bson.M{"$in": userIDs},
		},
	)
	return err
}

func (r *GroupUserRepository) SoftDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroupUser).UpdateMany(
		ctx,
		bson.M{
			"group._id": groupID,
			"deletedAt": bson.M{opExists: false},
		},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

func (r *GroupUserRepository) HardDeleteAllByGroup(ctx context.Context, groupID bson.ObjectID) error {
	_, err := r.db.Collection(collectionGroupUser).DeleteMany(
		ctx,
		bson.M{"group._id": groupID},
	)
	return err
}
