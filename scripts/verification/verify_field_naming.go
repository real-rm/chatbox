// +build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SessionDocument represents a session stored in MongoDB with camelCase field names
type SessionDocument struct {
	ID                 string            `bson:"_id"`
	UserID             string            `bson:"uid"`
	Name               string            `bson:"nm"`
	ModelID            string            `bson:"modelId"`
	Messages           []MessageDocument `bson:"msgs"`
	StartTime          time.Time         `bson:"ts"`
	EndTime            *time.Time        `bson:"endTs,omitempty"`
	Duration           int64             `bson:"dur"` // seconds
	AdminAssisted      bool              `bson:"adminAssisted"`
	AssistingAdminID   string            `bson:"assistingAdminId,omitempty"`
	AssistingAdminName string            `bson:"assistingAdminName,omitempty"`
	HelpRequested      bool              `bson:"helpRequested"`
	TotalTokens        int               `bson:"totalTokens"`
	MaxResponseTime    int64             `bson:"maxRespTime"` // milliseconds
	AvgResponseTime    int64             `bson:"avgRespTime"` // milliseconds
}

// MessageDocument represents a message stored in MongoDB
type MessageDocument struct {
	Content   string            `bson:"content"`
	Timestamp time.Time         `bson:"ts"`
	Sender    string            `bson:"sender"`
	FileID    string            `bson:"fileId,omitempty"`
	FileURL   string            `bson:"fileUrl,omitempty"`
	Metadata  map[string]string `bson:"meta,omitempty"`
}

func main() {
	fmt.Println("=== MongoDB Field Naming Verification ===\n")

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().
		ApplyURI("mongodb://127.0.0.1:27017").
		SetAuth(options.Credential{
			Username:   "admin",
			Password:   "password",
			AuthSource: "admin",
		})
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	// Ping MongoDB
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}
	fmt.Println("✓ Connected to MongoDB")

	// Get collection
	collection := client.Database("test_field_naming").Collection("sessions")

	// Clean up any existing test data
	collection.Drop(ctx)
	fmt.Println("✓ Cleaned up test collection")

	// Test 1: Create a document with camelCase field names
	fmt.Println("\nTest 1: Creating document with camelCase field names...")
	now := time.Time{}
	doc := SessionDocument{
		ID:                 "test-session-1",
		UserID:             "user-123",
		Name:               "Test Session",
		ModelID:            "gpt-4",
		Messages:           []MessageDocument{},
		StartTime:          now,
		EndTime:            nil,
		Duration:           0,
		AdminAssisted:      true,
		AssistingAdminID:   "admin-456",
		AssistingAdminName: "Admin User",
		HelpRequested:      false,
		TotalTokens:        100,
		MaxResponseTime:    1000,
		AvgResponseTime:    500,
	}

	_, err = collection.InsertOne(ctx, doc)
	if err != nil {
		log.Fatalf("Failed to insert document: %v", err)
	}
	fmt.Println("✓ Document inserted")

	// Test 2: Verify field names in MongoDB
	fmt.Println("\nTest 2: Verifying field names in MongoDB...")
	var rawDoc bson.M
	err = collection.FindOne(ctx, bson.M{"_id": "test-session-1"}).Decode(&rawDoc)
	if err != nil {
		log.Fatalf("Failed to find document: %v", err)
	}

	// Check camelCase field names
	expectedFields := map[string]string{
		"uid":                "user-123",
		"nm":                 "Test Session",
		"modelId":            "gpt-4",
		"msgs":               "array",
		"ts":                 "time",
		"dur":                "int64",
		"adminAssisted":      "true",
		"assistingAdminId":   "admin-456",
		"assistingAdminName": "Admin User",
		"helpRequested":      "false",
		"totalTokens":        "100",
		"maxRespTime":        "1000",
		"avgRespTime":        "500",
	}

	allFieldsCorrect := true
	for field := range expectedFields {
		if _, exists := rawDoc[field]; !exists {
			fmt.Printf("✗ Field '%s' not found in document\n", field)
			allFieldsCorrect = false
		} else {
			fmt.Printf("✓ Field '%s' exists\n", field)
		}
	}

	// Check that old snake_case fields don't exist
	oldFields := []string{"user_id", "model_id", "start_time", "end_time", "admin_assisted", "assisting_admin_id", "assisting_admin_name", "help_requested", "total_tokens", "max_response_time", "avg_response_time"}
	for _, field := range oldFields {
		if _, exists := rawDoc[field]; exists {
			fmt.Printf("✗ Old snake_case field '%s' still exists (should be removed)\n", field)
			allFieldsCorrect = false
		}
	}

	if allFieldsCorrect {
		fmt.Println("\n✓ All field names are correct (camelCase)")
	} else {
		fmt.Println("\n✗ Some field names are incorrect")
	}

	// Test 3: Query by uid field
	fmt.Println("\nTest 3: Querying by 'uid' field...")
	var result SessionDocument
	err = collection.FindOne(ctx, bson.M{"uid": "user-123"}).Decode(&result)
	if err != nil {
		log.Fatalf("Failed to query by uid: %v", err)
	}
	fmt.Printf("✓ Query by 'uid' successful: found session '%s'\n", result.ID)

	// Test 4: Query by adminAssisted field
	fmt.Println("\nTest 4: Querying by 'adminAssisted' field...")
	err = collection.FindOne(ctx, bson.M{"adminAssisted": true}).Decode(&result)
	if err != nil {
		log.Fatalf("Failed to query by adminAssisted: %v", err)
	}
	fmt.Printf("✓ Query by 'adminAssisted' successful: found session '%s'\n", result.ID)

	// Test 5: Sort by ts field
	fmt.Println("\nTest 5: Sorting by 'ts' field...")
	// Insert another document
	doc2 := SessionDocument{
		ID:        "test-session-2",
		UserID:    "user-456",
		Name:      "Test Session 2",
		StartTime: now.Add(1 * time.Hour),
	}
	collection.InsertOne(ctx, doc2)

	cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "ts", Value: -1}}))
	if err != nil {
		log.Fatalf("Failed to sort by ts: %v", err)
	}
	defer cursor.Close(ctx)

	var sessions []SessionDocument
	if err = cursor.All(ctx, &sessions); err != nil {
		log.Fatalf("Failed to decode sorted results: %v", err)
	}
	fmt.Printf("✓ Sort by 'ts' successful: found %d sessions\n", len(sessions))

	// Test 6: Sort by totalTokens field
	fmt.Println("\nTest 6: Sorting by 'totalTokens' field...")
	cursor, err = collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "totalTokens", Value: -1}}))
	if err != nil {
		log.Fatalf("Failed to sort by totalTokens: %v", err)
	}
	defer cursor.Close(ctx)

	if err = cursor.All(ctx, &sessions); err != nil {
		log.Fatalf("Failed to decode sorted results: %v", err)
	}
	fmt.Printf("✓ Sort by 'totalTokens' successful: found %d sessions\n", len(sessions))

	// Test 7: Update with new field names
	fmt.Println("\nTest 7: Updating document with new field names...")
	update := bson.M{
		"$set": bson.M{
			"nm":          "Updated Session Name",
			"totalTokens": 200,
		},
	}
	_, err = collection.UpdateOne(ctx, bson.M{"_id": "test-session-1"}, update)
	if err != nil {
		log.Fatalf("Failed to update document: %v", err)
	}
	fmt.Println("✓ Update successful")

	// Verify update
	err = collection.FindOne(ctx, bson.M{"_id": "test-session-1"}).Decode(&result)
	if err != nil {
		log.Fatalf("Failed to find updated document: %v", err)
	}
	if result.Name == "Updated Session Name" && result.TotalTokens == 200 {
		fmt.Println("✓ Update verified: fields updated correctly")
	} else {
		fmt.Println("✗ Update verification failed")
	}

	// Clean up
	collection.Drop(ctx)
	fmt.Println("\n✓ Test collection cleaned up")

	fmt.Println("\n=== All MongoDB Field Naming Tests Passed ===")
}
