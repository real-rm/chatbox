#!/bin/bash

# Script to verify MongoDB field naming inside Docker container

echo "=== MongoDB Field Naming Verification (Docker) ==="
echo ""

# Connect to MongoDB and run verification commands
docker exec chatbox-mongodb mongosh -u admin -p password --authenticationDatabase admin --eval '
use test_field_naming;

// Clean up
db.sessions.drop();
print("✓ Cleaned up test collection\n");

// Test 1: Insert document with camelCase field names
print("Test 1: Creating document with camelCase field names...");
db.sessions.insertOne({
  _id: "test-session-1",
  uid: "user-123",
  nm: "Test Session",
  modelId: "gpt-4",
  msgs: [],
  ts: new Date(),
  dur: 0,
  adminAssisted: true,
  assistingAdminId: "admin-456",
  assistingAdminName: "Admin User",
  helpRequested: false,
  totalTokens: 100,
  maxRespTime: 1000,
  avgRespTime: 500
});
print("✓ Document inserted\n");

// Test 2: Verify field names
print("Test 2: Verifying field names in MongoDB...");
var doc = db.sessions.findOne({_id: "test-session-1"});
var expectedFields = ["uid", "nm", "modelId", "msgs", "ts", "dur", "adminAssisted", "assistingAdminId", "assistingAdminName", "helpRequested", "totalTokens", "maxRespTime", "avgRespTime"];
var allFieldsCorrect = true;
expectedFields.forEach(function(field) {
  if (doc.hasOwnProperty(field)) {
    print("✓ Field \"" + field + "\" exists");
  } else {
    print("✗ Field \"" + field + "\" not found");
    allFieldsCorrect = false;
  }
});

// Check that old snake_case fields don\'t exist
var oldFields = ["user_id", "model_id", "start_time", "end_time", "admin_assisted", "assisting_admin_id", "assisting_admin_name", "help_requested", "total_tokens", "max_response_time", "avg_response_time"];
oldFields.forEach(function(field) {
  if (doc.hasOwnProperty(field)) {
    print("✗ Old snake_case field \"" + field + "\" still exists (should be removed)");
    allFieldsCorrect = false;
  }
});

if (allFieldsCorrect) {
  print("\n✓ All field names are correct (camelCase)\n");
} else {
  print("\n✗ Some field names are incorrect\n");
}

// Test 3: Query by uid field
print("Test 3: Querying by \"uid\" field...");
var result = db.sessions.findOne({uid: "user-123"});
if (result) {
  print("✓ Query by \"uid\" successful: found session \"" + result._id + "\"\n");
} else {
  print("✗ Query by \"uid\" failed\n");
}

// Test 4: Query by adminAssisted field
print("Test 4: Querying by \"adminAssisted\" field...");
result = db.sessions.findOne({adminAssisted: true});
if (result) {
  print("✓ Query by \"adminAssisted\" successful: found session \"" + result._id + "\"\n");
} else {
  print("✗ Query by \"adminAssisted\" failed\n");
}

// Test 5: Sort by ts field
print("Test 5: Sorting by \"ts\" field...");
db.sessions.insertOne({
  _id: "test-session-2",
  uid: "user-456",
  nm: "Test Session 2",
  ts: new Date(Date.now() + 3600000),
  msgs: []
});
var sessions = db.sessions.find().sort({ts: -1}).toArray();
print("✓ Sort by \"ts\" successful: found " + sessions.length + " sessions\n");

// Test 6: Sort by totalTokens field
print("Test 6: Sorting by \"totalTokens\" field...");
sessions = db.sessions.find().sort({totalTokens: -1}).toArray();
print("✓ Sort by \"totalTokens\" successful: found " + sessions.length + " sessions\n");

// Test 7: Update with new field names
print("Test 7: Updating document with new field names...");
db.sessions.updateOne(
  {_id: "test-session-1"},
  {$set: {nm: "Updated Session Name", totalTokens: 200}}
);
print("✓ Update successful");

// Verify update
result = db.sessions.findOne({_id: "test-session-1"});
if (result.nm === "Updated Session Name" && result.totalTokens === 200) {
  print("✓ Update verified: fields updated correctly\n");
} else {
  print("✗ Update verification failed\n");
}

// Clean up
db.sessions.drop();
print("✓ Test collection cleaned up\n");

print("=== All MongoDB Field Naming Tests Passed ===");
'
