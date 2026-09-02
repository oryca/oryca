package repository

import (
	"context"
	"net/url"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionPackageSvcLink = "package_services"

type PackageSvcLinkRepository struct {
	db *mongo.Database
}

func NewPackageSvcLinkRepository(db *mongo.Database) *PackageSvcLinkRepository {
	return &PackageSvcLinkRepository{db: db}
}

var packageSvcLinkSearchFields = []string{"package.name", "package.alias", "service.name", "service.description", "service.basePath"}

func (r *PackageSvcLinkRepository) Aggregate(ctx context.Context, packageID *bson.ObjectID, params url.Values) ([]*model.PackageSvcLinkDetail, int64, error) {
	pipeline := bson.A{}

	match := bson.M{}
	if packageID != nil {
		match["packageId"] = *packageID
	}
	pipeline = append(pipeline, bson.M{"$match": match})

	pipeline = append(pipeline, bson.M{"$lookup": bson.M{
		"from": collectionPackage, "localField": "packageId", "foreignField": "_id", "as": "package",
	}})
	pipeline = append(pipeline, bson.M{"$unwind": bson.M{"path": "$package", "preserveNullAndEmptyArrays": false}})

	pipeline = append(pipeline, bson.M{"$lookup": bson.M{
		"from": collectionGatewayService, "localField": "serviceId", "foreignField": "_id", "as": "service",
	}})
	pipeline = append(pipeline, bson.M{"$unwind": bson.M{"path": "$service", "preserveNullAndEmptyArrays": false}})

	pipeline = append(pipeline, bson.M{"$project": bson.M{
		"_id":   "$_id",
		"paths": "$paths",
		"package": bson.M{
			"_id":         "$package._id",
			"name":        "$package.name",
			"alias":       "$package.alias",
			"description": "$package.description",
		},
		"service": bson.M{
			"_id":         "$service._id",
			"name":        "$service.name",
			"description": "$service.description",
			"type":        "$service.type",
			"basePath":    "$service.basePath",
		},
		"createdAt": "$createdAt",
		"createdBy": "$createdBy",
		"updatedAt": "$updatedAt",
		"updatedBy": "$updatedBy",
	}})

	searchMatch := bson.M{}
	if s := params.Get("search"); s != "" {
		s = tool.EscapeRegexPattern(s)
		or := make([]bson.M, len(packageSvcLinkSearchFields))
		for i, f := range packageSvcLinkSearchFields {
			or[i] = tool.SearchWildcard(s, f)
		}
		searchMatch["$or"] = or
	}
	conditions, err := tool.GenerateFilterBson(params, tool.CommonIgnoredParams)
	if err != nil {
		return nil, 0, err
	}
	if len(conditions) > 0 {
		searchMatch["$and"] = conditions
	}
	if len(searchMatch) > 0 {
		pipeline = append(pipeline, bson.M{"$match": searchMatch})
	}

	if sort := params.Get("sort"); sort != "" {
		if sortBson, e := tool.OptionSortBson(sort); e == nil {
			pipeline = append(pipeline, bson.M{"$sort": sortBson})
		}
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

	pipeline = append(pipeline, bson.M{"$facet": bson.M{
		"count": bson.A{bson.M{"$count": "total"}},
		"data":  bson.A{bson.M{"$skip": offset}, bson.M{"$limit": limit}},
	}})

	cursor, err := r.db.Collection(collectionPackageSvcLink).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var aggResult []bson.M
	if err := cursor.All(ctx, &aggResult); err != nil {
		return nil, 0, err
	}

	var total int64
	var details []*model.PackageSvcLinkDetail

	if len(aggResult) > 0 {
		if countArr, ok := aggResult[0]["count"].(bson.A); ok && len(countArr) > 0 {
			if b, err := bson.Marshal(countArr[0]); err == nil {
				var cr struct {
					Total int32 `bson:"total"`
				}
				if err := bson.Unmarshal(b, &cr); err == nil {
					total = int64(cr.Total)
				}
			}
		}
		if dataArr, ok := aggResult[0]["data"].(bson.A); ok {
			for _, item := range dataArr {
				b, err := bson.Marshal(item)
				if err != nil {
					continue
				}
				var detail model.PackageSvcLinkDetail
				if err := bson.Unmarshal(b, &detail); err != nil {
					continue
				}
				details = append(details, &detail)
			}
		}
	}

	return details, total, nil
}

func (r *PackageSvcLinkRepository) FindByPackageID(ctx context.Context, packageID bson.ObjectID) ([]*model.PackageSvcLink, error) {
	cursor, err := r.db.Collection(collectionPackageSvcLink).Find(ctx, bson.M{"packageId": packageID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.PackageSvcLink
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *PackageSvcLinkRepository) FindByPackageAndService(ctx context.Context, packageID, serviceID bson.ObjectID) (*model.PackageSvcLink, error) {
	var result model.PackageSvcLink
	if err := r.db.Collection(collectionPackageSvcLink).FindOne(ctx, bson.M{
		"packageId": packageID,
		"serviceId": serviceID,
	}).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *PackageSvcLinkRepository) Insert(ctx context.Context, doc *model.PackageSvcLink) error {
	_, err := r.db.Collection(collectionPackageSvcLink).InsertOne(ctx, doc)
	return err
}

func (r *PackageSvcLinkRepository) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	_, err := r.db.Collection(collectionPackageSvcLink).UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
	)
	return err
}

func (r *PackageSvcLinkRepository) Delete(ctx context.Context, packageID, serviceID bson.ObjectID) error {
	_, err := r.db.Collection(collectionPackageSvcLink).DeleteOne(ctx, bson.M{
		"packageId": packageID,
		"serviceId": serviceID,
	})
	return err
}

// FindByServiceID returns all PackageSvcLinks for a service including Paths per package
func (r *PackageSvcLinkRepository) FindByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]*model.PackageSvcLink, error) {
	cursor, err := r.db.Collection(collectionPackageSvcLink).Find(ctx, bson.M{"serviceId": serviceID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*model.PackageSvcLink
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *PackageSvcLinkRepository) FindPackageIDsByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]string, error) {
	cursor, err := r.db.Collection(collectionPackageSvcLink).Find(
		ctx,
		bson.M{"serviceId": serviceID},
		options.Find().SetProjection(bson.M{"packageId": 1}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		PackageID bson.ObjectID `bson:"packageId"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.PackageID.Hex())
	}
	return ids, nil
}
