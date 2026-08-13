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

const (
	collectionMailServer = "system_mail_servers"
	opExists             = "$exists"
)

type MailServerRepository struct {
	db *mongo.Database
}

func NewMailServerRepository(db *mongo.Database) *MailServerRepository {
	return &MailServerRepository{db: db}
}

var mailServerSearchFields = []string{"name", "sender", "senderEmail"}

func (r *MailServerRepository) buildFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(mailServerSearchFields))
		for i, f := range mailServerSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *MailServerRepository) FindAll(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionMailServer).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionMailServer).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.MailServer
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *MailServerRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error) {
	filter := bson.M{
		"_id": id,
		"deletedAt": bson.M{
			opExists: false,
		},
	}

	var result model.MailServer
	err := r.db.Collection(collectionMailServer).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *MailServerRepository) FindDefault(ctx context.Context) (*model.MailServer, error) {
	filter := bson.M{
		"default": true,
		"deletedAt": bson.M{
			opExists: false,
		},
	}

	var result model.MailServer
	err := r.db.Collection(collectionMailServer).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *MailServerRepository) Insert(ctx context.Context, doc *model.MailServer) error {
	_, err := r.db.Collection(collectionMailServer).InsertOne(ctx, doc)
	return err
}

func (r *MailServerRepository) Update(ctx context.Context, id bson.ObjectID, doc *model.MailServer) error {
	opts := options.UpdateOne()
	_, err := r.db.Collection(collectionMailServer).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": doc},
		opts,
	)

	return err
}

// UnsetDefault ล้าง default=true ของทุก document (ก่อน set default ใหม่)
func (r *MailServerRepository) UnsetDefault(ctx context.Context) error {
	_, err := r.db.Collection(collectionMailServer).UpdateMany(
		ctx,
		bson.M{"default": true, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"default": false}},
	)
	return err
}

// SoftDelete ตั้ง deletedAt แต่ไม่ลบออกจาก DB
func (r *MailServerRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionMailServer).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)

	return err
}

// HardDelete ลบออกจาก DB ถาวร
func (r *MailServerRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionMailServer).DeleteOne(ctx, bson.M{"_id": id})
	return err
}
