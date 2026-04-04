package pooling

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ConnectMongo(uri string, maxOpen, maxIdle, timeoutSeconds int) (*mongo.Client, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}

	opts := options.Client().ApplyURI(uri)
	if maxOpen > 0 {
		opts.SetMaxPoolSize(uint64(maxOpen))
	}
	if maxIdle > 0 {
		opts.SetMinPoolSize(uint64(maxIdle))
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	opts.SetConnectTimeout(timeout)
	opts.SetServerSelectionTimeout(timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}
