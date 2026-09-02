package repository

import (
	"context"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionUserSession = "user_sessions"

type UserSessionRepository struct {
	db *mongo.Database
}

func NewUserSessionRepository(db *mongo.Database) *UserSessionRepository {
	return &UserSessionRepository{db: db}
}

func (r *UserSessionRepository) col() *mongo.Collection {
	return r.db.Collection(collectionUserSession)
}

func (r *UserSessionRepository) Insert(ctx context.Context, doc *model.UserSession) error {
	_, err := r.col().InsertOne(ctx, doc)
	return err
}

func (r *UserSessionRepository) UpdateLogout(ctx context.Context, sessionID bson.ObjectID, logoutAt time.Time, reason string) error {
	filter := bson.M{"_id": sessionID}
	update := bson.M{"$set": bson.M{
		"logoutAt":     logoutAt,
		"logoutReason": reason,
	}}
	_, err := r.col().UpdateOne(ctx, filter, update)
	return err
}

func (r *UserSessionRepository) BulkUpdateLogout(ctx context.Context, sessionIDs []bson.ObjectID, logoutAt time.Time, reason string) error {
	filter := bson.M{"_id": bson.M{"$in": sessionIDs}}
	update := bson.M{"$set": bson.M{
		"logoutAt":     logoutAt,
		"logoutReason": reason,
	}}
	_, err := r.col().UpdateMany(ctx, filter, update)
	return err
}

// FindByIDs returns sessions matching the given IDs.
func (r *UserSessionRepository) FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.UserSession, error) {
	filter := bson.M{"_id": bson.M{"$in": ids}}
	cursor, err := r.col().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []*model.UserSession
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// FindHistoryByUserID returns all sessions for a user sorted by loginAt descending.
func (r *UserSessionRepository) FindHistoryByUserID(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error) {
	filter := bson.M{"userId": userID}
	opts := options.Find().SetSort(bson.D{{Key: "loginAt", Value: -1}})
	cursor, err := r.col().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []*model.UserSession
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// FindActiveByUserID returns sessions where logoutAt does not exist (still active).
func (r *UserSessionRepository) FindActiveByUserID(ctx context.Context, userID bson.ObjectID) ([]*model.UserSession, error) {
	filter := bson.M{
		"userId":   userID,
		"logoutAt": bson.M{opExists: false},
	}
	cursor, err := r.col().Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []*model.UserSession
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}
