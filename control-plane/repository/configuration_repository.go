package repository

import (
	"context"

	"github.com/oryca/oryca/control-plane/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionConfiguration = "system_configurations"

type ConfigurationRepository struct {
	db *mongo.Database
}

func NewConfigurationRepository(db *mongo.Database) *ConfigurationRepository {
	return &ConfigurationRepository{db: db}
}

func (r *ConfigurationRepository) Get(ctx context.Context) (*model.Configuration, error) {
	var result model.Configuration
	err := r.db.Collection(collectionConfiguration).FindOne(ctx, bson.M{}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ConfigurationRepository) Upsert(ctx context.Context, doc *model.Configuration) error {
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.db.Collection(collectionConfiguration).UpdateOne(
		ctx,
		bson.M{},
		bson.M{"$set": doc},
		opts,
	)
	return err
}
