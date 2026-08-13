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

const collectionNotification = "notifications"

const notificationTTL = 30 * 24 * time.Hour

type NotificationRepository struct {
	db *mongo.Database
}

func NewNotificationRepository(db *mongo.Database) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.db.Collection(collectionNotification).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "userId", Value: 1}, {Key: "createdAt", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "userId", Value: 1}, {Key: "read", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "dedupKey", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys:    bson.D{{Key: "createdAt", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(notificationTTL.Seconds())),
		},
	})
	return err
}

func (r *NotificationRepository) Insert(ctx context.Context, doc *model.Notification) error {
	_, err := r.db.Collection(collectionNotification).InsertOne(ctx, doc)
	return err
}

// IsDuplicateKeyError คืน true ถ้า err มาจาก dedupKey ชนกับ unique index
func IsDuplicateKeyError(err error) bool {
	return mongo.IsDuplicateKeyError(err)
}

// buildFilter สร้าง filter มาตรฐานเดียวกับ repo อื่น (ผ่าน tool.GenerateFilterBson รองรับทุก field,
// wildcard, exclude, multi-value) — userId เป็น base filter เสมอ กัน query param override เห็นของคนอื่น
func (r *NotificationRepository) buildFilter(userID bson.ObjectID, params url.Values) (bson.M, error) {
	filter := bson.M{"userId": userID}

	if len(params) > 0 {
		conditions, err := tool.GenerateFilterBson(params, tool.CommonIgnoredParams)
		if err != nil {
			return nil, err
		}
		if len(conditions) > 0 {
			filter["$and"] = conditions
		}
	}

	return filter, nil
}

func (r *NotificationRepository) FindAllByUser(ctx context.Context, userID bson.ObjectID, params url.Values) ([]*model.Notification, int64, error) {
	filter, err := r.buildFilter(userID, params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionNotification).CountDocuments(ctx, filter)
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
	} else {
		findOpts.SetSort(bson.D{{Key: "createdAt", Value: -1}})
	}

	cursor, err := r.db.Collection(collectionNotification).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.Notification
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID bson.ObjectID) (int64, error) {
	return r.db.Collection(collectionNotification).CountDocuments(ctx, bson.M{"userId": userID, "read": false})
}

// MarkAsRead — กรองคู่ userId เสมอ กัน IDOR
func (r *NotificationRepository) MarkAsRead(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error {
	now := tool.NowUTC()
	res, err := r.db.Collection(collectionNotification).UpdateOne(
		ctx,
		bson.M{"_id": id, "userId": userID},
		bson.M{"$set": bson.M{"read": true, "readAt": now}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, userID bson.ObjectID) error {
	now := tool.NowUTC()
	_, err := r.db.Collection(collectionNotification).UpdateMany(
		ctx,
		bson.M{"userId": userID, "read": false},
		bson.M{"$set": bson.M{"read": true, "readAt": now}},
	)
	return err
}

// Delete — กรองคู่ userId เสมอ กัน IDOR
func (r *NotificationRepository) Delete(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error {
	res, err := r.db.Collection(collectionNotification).DeleteOne(ctx, bson.M{"_id": id, "userId": userID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
