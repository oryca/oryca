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

const collectionEmailTemplate = "system_email_templates"

type EmailTemplateRepository struct {
	db *mongo.Database
}

func NewEmailTemplateRepository(db *mongo.Database) *EmailTemplateRepository {
	return &EmailTemplateRepository{db: db}
}

var emailTemplateSearchFields = []string{"name", "alias", "subject"}

func (r *EmailTemplateRepository) buildFilter(params url.Values) (bson.M, error) {
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
		or := make([]bson.M, len(emailTemplateSearchFields))
		for i, f := range emailTemplateSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		filter["$or"] = or
	}
	return filter, nil
}

func (r *EmailTemplateRepository) FindAll(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error) {
	filter, err := r.buildFilter(params)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.db.Collection(collectionEmailTemplate).CountDocuments(ctx, filter)
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

	cursor, err := r.db.Collection(collectionEmailTemplate).Find(ctx, filter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var results []*model.EmailTemplate
	if err := cursor.All(ctx, &results); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *EmailTemplateRepository) FindByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error) {
	filter := bson.M{"_id": id, "deletedAt": bson.M{opExists: false}}
	var result model.EmailTemplate
	if err := r.db.Collection(collectionEmailTemplate).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *EmailTemplateRepository) FindByAlias(ctx context.Context, alias string) (*model.EmailTemplate, error) {
	filter := bson.M{"alias": alias, "deletedAt": bson.M{opExists: false}}
	var result model.EmailTemplate
	if err := r.db.Collection(collectionEmailTemplate).FindOne(ctx, filter).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *EmailTemplateRepository) Insert(ctx context.Context, doc *model.EmailTemplate) error {
	_, err := r.db.Collection(collectionEmailTemplate).InsertOne(ctx, doc)
	return err
}

func (r *EmailTemplateRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionEmailTemplate).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": fields},
	)
	return err
}

// UnsetDefault ล้าง default=true ใน type เดียวกัน (ก่อน set default ใหม่)
func (r *EmailTemplateRepository) UnsetDefault(ctx context.Context, templateType string) error {
	_, err := r.db.Collection(collectionEmailTemplate).UpdateMany(
		ctx,
		bson.M{"type": templateType, "default": true, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"default": false}},
	)
	return err
}

// SoftDelete ตั้ง deletedAt แต่ไม่ลบออกจาก DB
func (r *EmailTemplateRepository) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	_, err := r.db.Collection(collectionEmailTemplate).UpdateOne(
		ctx,
		bson.M{"_id": id, "deletedAt": bson.M{opExists: false}},
		bson.M{"$set": bson.M{"deletedAt": deletedAt, "deletedBy": deletedBy}},
	)
	return err
}

// HardDelete ลบออกจาก DB ถาวร
func (r *EmailTemplateRepository) HardDelete(ctx context.Context, id bson.ObjectID) error {
	_, err := r.db.Collection(collectionEmailTemplate).DeleteOne(ctx, bson.M{"_id": id})
	return err
}
