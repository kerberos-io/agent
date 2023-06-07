package database

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DB struct {
	Client *mongo.Client
}

var _init_ctx sync.Once
var _instance *DB
var DatabaseName = "KerberosFactory"

func New() *mongo.Client {

	host := os.Getenv("MONGODB_HOST")
	databaseCredentials := os.Getenv("MONGODB_DATABASE_CREDENTIALS")
	replicaset := os.Getenv("MONGODB_REPLICASET")
	username := os.Getenv("MONGODB_USERNAME")
	password := os.Getenv("MONGODB_PASSWORD")
	authentication := "SCRAM-SHA-256"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_init_ctx.Do(func() {
		_instance = new(DB)
		mongodbURI := fmt.Sprintf("mongodb://%s:%s@%s", username, password, host)
		if replicaset != "" {
			mongodbURI = fmt.Sprintf("%s/?replicaSet=%s", mongodbURI, replicaset)
		}

		client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongodbURI).SetAuth(options.Credential{
			AuthMechanism: authentication,
			AuthSource:    databaseCredentials,
			Username:      username,
			Password:      password,
		}))
		if err != nil {
			fmt.Printf("Error setting up mongodb connection: %+v\n", err)
			os.Exit(1)
		}
		_instance.Client = client
	})

	return _instance.Client
}
