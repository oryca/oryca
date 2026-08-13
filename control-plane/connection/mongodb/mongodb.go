package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func Connect(uri string) (*mongo.Client, error) {
	clientOptions := options.Client().
		ApplyURI(uri).
		// Connection Pool
		SetMaxPoolSize(50).
		SetMinPoolSize(10).
		SetMaxConnIdleTime(5 * time.Minute).
		SetMaxConnecting(10).
		// Timeouts
		SetConnectTimeout(5 * time.Second).
		SetServerSelectionTimeout(3 * time.Second).
		// Retry
		SetRetryWrites(true).
		SetRetryReads(true).
		// Monitoring
		SetHeartbeatInterval(10 * time.Second)

	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return client, nil
}
