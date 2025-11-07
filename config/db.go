// config/db.go
package config

import (
	"context"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectDB establishes connection to MongoDB
func ConnectDB() *mongo.Client {
	// Set client options
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		// Default to localhost for local development
		// In Docker, this will be overridden by the MONGO_URI environment variable
		mongoURI = "mongodb://admin:9Z9ZBarrim@localhost:27017/?authSource=admin"
	}

	clientOptions := options.Client().ApplyURI(mongoURI)

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal("MongoDB connection error:", err)
	}

	// Check the connection
	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal("MongoDB ping error:", err)
	}

	log.Println("Connected to MongoDB")

	// Setup necessary collections and indexes
	setupCollections(client)

	return client
}

// GetCollection returns MongoDB collection
func GetCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "barrim"
	}
	return client.Database(dbName).Collection(collectionName)
}

// setupCollections ensures all necessary collections and indexes exist
func setupCollections(client *mongo.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "barrim"
	}

	db := client.Database(dbName)

	// Ensure collections exist
	collections := []string{"users", "companies", "serviceProviders", "wholesalers"}
	for _, collName := range collections {
		db.CreateCollection(ctx, collName)
	}

	// Create indexes for faster lookups

	// Email index for users collection
	userColl := db.Collection("users")
	emailIndexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := userColl.Indexes().CreateOne(ctx, emailIndexModel)
	if err != nil {
		log.Printf("Error creating email index: %v", err)
	}

	// UserId index for entity collections
	for _, collName := range []string{"companies", "serviceProviders", "wholesalers"} {
		coll := db.Collection(collName)
		userIdIndexModel := mongo.IndexModel{
			Keys:    bson.D{{Key: "userId", Value: 1}},
			Options: options.Index().SetUnique(true),
		}
		_, err := coll.Indexes().CreateOne(ctx, userIdIndexModel)
		if err != nil {
			log.Printf("Error creating userId index for %s: %v", collName, err)
		}
	}

	log.Println("Database collections and indexes setup complete")
}
