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

var TIMEOUT = 10 * time.Second
var _init_ctx sync.Once
var _instance *DB

var DatabaseName = os.Getenv("MONGODB_DATABASE_FACTORY")

func New() *DB {

	if DatabaseName == "" {
		DatabaseName = "KerberosFactory"
	}

	mongodbURI := os.Getenv("MONGODB_URI")
	host := os.Getenv("MONGODB_HOST")
	databaseCredentials := os.Getenv("MONGODB_DATABASE_CREDENTIALS")
	replicaset := os.Getenv("MONGODB_REPLICASET")
	username := os.Getenv("MONGODB_USERNAME")
	password := os.Getenv("MONGODB_PASSWORD")
	authentication := "SCRAM-SHA-256"

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	_init_ctx.Do(func() {
		_instance = new(DB)

		// We can also apply the complete URI
		// e.g. "mongodb+srv://<username>:<password>@kerberos-hub.shhng.mongodb.net/?retryWrites=true&w=majority&appName=kerberos-hub"
		if mongodbURI != "" {
			serverAPI := options.ServerAPI(options.ServerAPIVersion1)
			opts := options.Client().ApplyURI(mongodbURI).SetServerAPIOptions(serverAPI)

			// Create a new client and connect to the server
			client, err := mongo.Connect(ctx, opts)
			if err != nil {
				fmt.Printf("Error setting up mongodb connection: %+v\n", err)
				os.Exit(1)
			}
			_instance.Client = client

		} else {

			// New MongoDB driver
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
		}
	})

	return _instance
}
