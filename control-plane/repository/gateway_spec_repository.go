package repository

import (
	"context"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const collectionGatewaySpec = "gateway_specs"

type GatewaySpecRepository struct {
	db *mongo.Database
}

func NewGatewaySpecRepository(db *mongo.Database) *GatewaySpecRepository {
	return &GatewaySpecRepository{db: db}
}

func (r *GatewaySpecRepository) FindByServiceID(ctx context.Context, serviceID bson.ObjectID) (*model.GatewaySpec, error) {
	var result model.GatewaySpec
	if err := r.db.Collection(collectionGatewaySpec).FindOne(ctx, bson.M{"serviceId": serviceID}).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *GatewaySpecRepository) Insert(ctx context.Context, doc *model.GatewaySpec) error {
	_, err := r.db.Collection(collectionGatewaySpec).InsertOne(ctx, doc)
	return err
}

func (r *GatewaySpecRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionGatewaySpec).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
	)
	return err
}

func (r *GatewaySpecRepository) Delete(ctx context.Context, serviceID bson.ObjectID) error {
	_, err := r.db.Collection(collectionGatewaySpec).DeleteOne(ctx, bson.M{"serviceId": serviceID})
	return err
}
