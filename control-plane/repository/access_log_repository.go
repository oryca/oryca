package repository

import (
	"context"
	"errors"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionAccessLog = "access_logs"

const indexAccessLogTTL = "access_log_ttl"

// Bucket intervals for the dashboard chart, picked from the range length so a
// long window does not return one point per minute.
const (
	IntervalMinute = "minute"
	IntervalHour   = "hour"
	IntervalDay    = "day"
)

type AccessLogRepository struct {
	db *mongo.Database
}

func NewAccessLogRepository(db *mongo.Database) *AccessLogRepository {
	return &AccessLogRepository{db: db}
}

// EnsureIndexes creates the query indexes and the TTL index that expires old
// logs. Mongo refuses to change expireAfterSeconds through CreateOne, so a
// changed retention is applied with collMod instead — otherwise editing the env
// var would silently keep the old retention forever.
func (r *AccessLogRepository) EnsureIndexes(ctx context.Context, retention time.Duration) error {
	coll := r.db.Collection(collectionAccessLog)
	expireSeconds := int32(retention.Seconds())

	if _, err := coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "userId", Value: 1}, {Key: "time", Value: -1}}},
		{Keys: bson.D{{Key: "serviceId", Value: 1}, {Key: "time", Value: -1}}},
		{Keys: bson.D{{Key: "time", Value: -1}, {Key: "_id", Value: -1}}},
	}); err != nil {
		return err
	}

	// TTL แยกจากก้อนบน: ถ้า retention เปลี่ยน CreateOne จะ error IndexOptionsConflict
	// (Mongo แก้ expireAfterSeconds ผ่าน createIndexes ไม่ได้) ต้องใช้ collMod แทน
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "time", Value: 1}},
		Options: options.Index().SetName(indexAccessLogTTL).SetExpireAfterSeconds(expireSeconds),
	})
	if err == nil {
		return nil
	}
	if !isIndexOptionsConflict(err) {
		return err
	}

	return r.db.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: collectionAccessLog},
		{Key: "index", Value: bson.D{
			{Key: "name", Value: indexAccessLogTTL},
			{Key: "expireAfterSeconds", Value: expireSeconds},
		}},
	}).Err()
}

// isIndexOptionsConflict คือกรณี index ชื่อเดิมมีอยู่แล้วแต่ option ต่าง (เช่น retention เปลี่ยน)
func isIndexOptionsConflict(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code == 85 // IndexOptionsConflict
	}
	return false
}

// InsertMany writes a batch unordered: one bad document must not drop the rest
// of the batch, since the consumer acks the whole batch together.
func (r *AccessLogRepository) InsertMany(ctx context.Context, docs []*model.AccessLog) error {
	if len(docs) == 0 {
		return nil
	}
	items := make([]any, 0, len(docs))
	for _, d := range docs {
		items = append(items, d)
	}
	_, err := r.db.Collection(collectionAccessLog).InsertMany(ctx, items, options.InsertMany().SetOrdered(false))
	return err
}

// buildFilter turns a query into a Mongo filter. Callers must have already
// applied scoping to q.UserID — this function trusts what it is given.
func buildAccessLogFilter(q model.AccessLogQuery) bson.M {
	timeFilter := bson.M{}
	if !q.From.IsZero() {
		timeFilter["$gte"] = q.From
	}
	if !q.To.IsZero() {
		timeFilter["$lte"] = q.To
	}

	filter := bson.M{}
	if len(timeFilter) > 0 {
		filter["time"] = timeFilter
	}
	if q.UserID != "" {
		filter["userId"] = q.UserID
	}
	if q.ServiceID != "" {
		filter["serviceId"] = q.ServiceID
	}
	return filter
}

// statusFacets counts 2xx/4xx/5xx in one pass instead of three queries.
func statusFacets() bson.D {
	return bson.D{
		{Key: "count", Value: bson.M{"$sum": 1}},
		{Key: "avgTimeMs", Value: bson.M{"$avg": "$durationMs"}},
		{Key: "status2xx", Value: bson.M{"$sum": bson.M{"$cond": bson.A{
			bson.M{"$and": bson.A{
				bson.M{"$gte": bson.A{"$statusCode", 200}},
				bson.M{"$lt": bson.A{"$statusCode", 300}},
			}}, 1, 0,
		}}}},
		{Key: "status4xx", Value: bson.M{"$sum": bson.M{"$cond": bson.A{
			bson.M{"$and": bson.A{
				bson.M{"$gte": bson.A{"$statusCode", 400}},
				bson.M{"$lt": bson.A{"$statusCode", 500}},
			}}, 1, 0,
		}}}},
		{Key: "status5xx", Value: bson.M{"$sum": bson.M{"$cond": bson.A{
			bson.M{"$gte": bson.A{"$statusCode", 500}}, 1, 0,
		}}}},
	}
}

// Stats returns the request volume bucketed at interval.
func (r *AccessLogRepository) Stats(ctx context.Context, q model.AccessLogQuery, interval string) ([]*model.DashboardBucket, error) {
	group := append(bson.D{
		{Key: "_id", Value: bson.M{"$dateTrunc": bson.M{"date": "$time", "unit": interval}}},
	}, statusFacets()...)

	cursor, err := r.db.Collection(collectionAccessLog).Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: buildAccessLogFilter(q)}},
		bson.D{{Key: "$group", Value: group}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		Time      time.Time `bson:"_id"`
		Count     int64     `bson:"count"`
		AvgTimeMs float64   `bson:"avgTimeMs"`
		Status2xx int64     `bson:"status2xx"`
		Status4xx int64     `bson:"status4xx"`
		Status5xx int64     `bson:"status5xx"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	buckets := make([]*model.DashboardBucket, 0, len(rows))
	for _, row := range rows {
		buckets = append(buckets, &model.DashboardBucket{
			Time:      row.Time,
			Count:     row.Count,
			AvgTimeMs: row.AvgTimeMs,
			Status2xx: row.Status2xx,
			Status4xx: row.Status4xx,
			Status5xx: row.Status5xx,
		})
	}
	return buckets, nil
}

// Summary totals the whole range in one aggregation.
func (r *AccessLogRepository) Summary(ctx context.Context, q model.AccessLogQuery) (*model.DashboardSummary, error) {
	group := append(bson.D{{Key: "_id", Value: nil}}, statusFacets()...)

	cursor, err := r.db.Collection(collectionAccessLog).Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: buildAccessLogFilter(q)}},
		bson.D{{Key: "$group", Value: group}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		Count     int64   `bson:"count"`
		AvgTimeMs float64 `bson:"avgTimeMs"`
		Status2xx int64   `bson:"status2xx"`
		Status4xx int64   `bson:"status4xx"`
		Status5xx int64   `bson:"status5xx"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	summary := &model.DashboardSummary{TopServices: []*model.DashboardServiceUsage{}}
	if len(rows) > 0 {
		summary.Total = rows[0].Count
		summary.AvgTimeMs = rows[0].AvgTimeMs
		summary.Status2xx = rows[0].Status2xx
		summary.Status4xx = rows[0].Status4xx
		summary.Status5xx = rows[0].Status5xx
	}
	return summary, nil
}

// TopServices returns the busiest services in the range, most requests first.
func (r *AccessLogRepository) TopServices(ctx context.Context, q model.AccessLogQuery, limit int) ([]*model.DashboardServiceUsage, error) {
	filter := buildAccessLogFilter(q)
	filter["serviceId"] = bson.M{"$nin": bson.A{"", nil}}
	if q.ServiceID != "" {
		filter["serviceId"] = q.ServiceID
	}

	cursor, err := r.db.Collection(collectionAccessLog).Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$serviceId"},
			{Key: "count", Value: bson.M{"$sum": 1}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit}},
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		ServiceID string `bson:"_id"`
		Count     int64  `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	out := make([]*model.DashboardServiceUsage, 0, len(rows))
	for _, row := range rows {
		out = append(out, &model.DashboardServiceUsage{ServiceID: row.ServiceID, Count: row.Count})
	}
	return out, nil
}

// FindLogs returns raw logs newest first, paginated by a (time, _id) cursor
// rather than skip/limit so paging stays correct while new logs arrive.
func (r *AccessLogRepository) FindLogs(ctx context.Context, q model.AccessLogQuery) ([]*model.AccessLog, error) {
	filter := buildAccessLogFilter(q)
	if q.CursorTime != nil && q.CursorID != nil {
		filter["$or"] = bson.A{
			bson.M{"time": bson.M{"$lt": *q.CursorTime}},
			bson.M{"time": *q.CursorTime, "_id": bson.M{"$lt": *q.CursorID}},
		}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "time", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(q.Limit))

	cursor, err := r.db.Collection(collectionAccessLog).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*model.AccessLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}
