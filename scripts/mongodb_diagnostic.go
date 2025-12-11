package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load MongoDB URI from env
	mongoURL := os.Getenv("MONGODB_URI")
	if mongoURL == "" {
		fmt.Println("Error: MONGODB_URI not set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		fmt.Printf("Failed to connect to MongoDB: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	db := client.Database(os.Getenv("MONGODB_DATABASE"))
	if db == nil {
		fmt.Println("Error: database not found")
		os.Exit(1)
	}

	collection := db.Collection(os.Getenv("MONGODB_COLLECTION"))

	// Count total documents
	total, err := collection.EstimatedDocumentCount(ctx)
	if err != nil {
		fmt.Printf("Error counting documents: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n=== MongoDB Events Collection Diagnostic ===\n")
	fmt.Printf("Total Events: %d\n\n", total)

	// Count events by client_id
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$client_id",
				"count": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"count": -1},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		fmt.Printf("Error aggregating: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Events by Client:")
	fmt.Println("================")
	var results []bson.M
	if err = cursor.All(ctx, &results); err != nil {
		fmt.Printf("Error decoding results: %v\n", err)
		os.Exit(1)
	}

	for _, result := range results {
		clientID := result["_id"]
		count := result["count"]
		fmt.Printf("  %v: %v events\n", clientID, count)
	}

	// Count events by event type for each client
	fmt.Println("\nEvents by Type per Client:")
	fmt.Println("==========================")

	pipeline2 := []bson.M{
		{
			"$group": bson.M{
				"_id": bson.M{
					"client_id": "$client_id",
					"event":     "$event",
				},
				"count": bson.M{"$sum": 1},
			},
		},
		{
			"$sort": bson.M{"_id.client_id": 1, "count": -1},
		},
	}

	cursor2, err := collection.Aggregate(ctx, pipeline2)
	if err != nil {
		fmt.Printf("Error aggregating: %v\n", err)
		os.Exit(1)
	}

	var results2 []bson.M
	if err = cursor2.All(ctx, &results2); err != nil {
		fmt.Printf("Error decoding results: %v\n", err)
		os.Exit(1)
	}

	var currentClient string
	for _, result := range results2 {
		id := result["_id"].(bson.M)
		clientID := id["client_id"]
		eventType := id["event"]
		count := result["count"]

		if clientID != currentClient {
			fmt.Printf("\n%v:\n", clientID)
			currentClient = fmt.Sprintf("%v", clientID)
		}
		fmt.Printf("  %v: %v\n", eventType, count)
	}

	// Show latest event timestamps
	fmt.Println("\nLatest Event per Client:")
	fmt.Println("========================")

	pipeline3 := []bson.M{
		{
			"$sort": bson.M{"received_at": -1},
		},
		{
			"$group": bson.M{
				"_id":          "$client_id",
				"latest_event": bson.M{"$first": "$received_at"},
				"event_type":   bson.M{"$first": "$event"},
			},
		},
	}

	cursor3, err := collection.Aggregate(ctx, pipeline3)
	if err != nil {
		fmt.Printf("Error aggregating: %v\n", err)
		os.Exit(1)
	}

	var results3 []bson.M
	if err = cursor3.All(ctx, &results3); err != nil {
		fmt.Printf("Error decoding results: %v\n", err)
		os.Exit(1)
	}

	for _, result := range results3 {
		clientID := result["_id"]
		latestEvent := result["latest_event"]
		eventType := result["event_type"]
		fmt.Printf("  %v: %v (type: %v)\n", clientID, latestEvent, eventType)
	}
}
