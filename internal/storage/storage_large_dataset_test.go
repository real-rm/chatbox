package storage

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/real-rm/chatbox/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSortByMessageCount_LargeDataset tests sorting correctness with large datasets
// This validates that the O(n log n) sort.Slice implementation correctly sorts
// large datasets as required by task 11.3
func TestSortByMessageCount_LargeDataset(t *testing.T) {
	testCases := []struct {
		name      string
		size      int
		ascending bool
	}{
		{"1000_sessions_ascending", 1000, true},
		{"1000_sessions_descending", 1000, false},
		{"5000_sessions_ascending", 5000, true},
		{"5000_sessions_descending", 5000, false},
		{"10000_sessions_ascending", 10000, true},
		{"10000_sessions_descending", 10000, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create sessions with random message counts
			sessions := make([]*SessionMetadata, tc.size)
			rand.Seed(time.Now().UnixNano())

			for i := 0; i < tc.size; i++ {
				sessions[i] = &SessionMetadata{
					ID:           fmt.Sprintf("session-%d", i),
					UserID:       fmt.Sprintf("user-%d", i%100),
					Name:         fmt.Sprintf("Session %d", i),
					StartTime:    time.Now().Add(-time.Duration(i) * time.Hour),
					MessageCount: rand.Intn(1000), // Random message count 0-999
					TotalTokens:  i * 10,
				}
			}

			// Sort the sessions
			start := time.Now()
			sortByMessageCount(sessions, tc.ascending)
			elapsed := time.Since(start)

			// Verify sorting is correct
			for i := 1; i < len(sessions); i++ {
				if tc.ascending {
					assert.True(t, sessions[i-1].MessageCount <= sessions[i].MessageCount,
						"Sessions not sorted correctly at index %d: %d > %d",
						i, sessions[i-1].MessageCount, sessions[i].MessageCount)
				} else {
					assert.True(t, sessions[i-1].MessageCount >= sessions[i].MessageCount,
						"Sessions not sorted correctly at index %d: %d < %d",
						i, sessions[i-1].MessageCount, sessions[i].MessageCount)
				}
			}

			// Verify performance is acceptable (should be sub-second for 10k items)
			assert.True(t, elapsed < time.Second,
				"Sorting %d sessions took %v, expected < 1s", tc.size, elapsed)

			t.Logf("Sorted %d sessions in %v", tc.size, elapsed)
		})
	}
}

// TestSortByMessageCount_LargeDataset_EdgeCases tests edge cases with large datasets
func TestSortByMessageCount_LargeDataset_EdgeCases(t *testing.T) {
	t.Run("AllSameMessageCount", func(t *testing.T) {
		size := 10000
		sessions := make([]*SessionMetadata, size)

		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				MessageCount: 100, // All same
			}
		}

		sortByMessageCount(sessions, true)

		// Verify all still have same count
		for i := 0; i < len(sessions); i++ {
			assert.Equal(t, 100, sessions[i].MessageCount)
		}
	})

	t.Run("AlreadySorted", func(t *testing.T) {
		size := 10000
		sessions := make([]*SessionMetadata, size)

		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				MessageCount: i, // Already sorted ascending
			}
		}

		start := time.Now()
		sortByMessageCount(sessions, true)
		elapsed := time.Since(start)

		// Verify still sorted
		for i := 1; i < len(sessions); i++ {
			assert.True(t, sessions[i-1].MessageCount <= sessions[i].MessageCount)
		}

		// Should be fast even for already sorted data
		assert.True(t, elapsed < time.Second)
		t.Logf("Sorted %d already-sorted sessions in %v", size, elapsed)
	})

	t.Run("ReverseSorted", func(t *testing.T) {
		size := 10000
		sessions := make([]*SessionMetadata, size)

		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				MessageCount: size - i, // Reverse sorted
			}
		}

		start := time.Now()
		sortByMessageCount(sessions, true)
		elapsed := time.Since(start)

		// Verify now sorted ascending
		for i := 1; i < len(sessions); i++ {
			assert.True(t, sessions[i-1].MessageCount <= sessions[i].MessageCount)
		}

		assert.True(t, elapsed < time.Second)
		t.Logf("Sorted %d reverse-sorted sessions in %v", size, elapsed)
	})

	t.Run("ManyDuplicates", func(t *testing.T) {
		size := 10000
		sessions := make([]*SessionMetadata, size)

		// Create sessions with only 10 different message counts
		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				MessageCount: i % 10, // Only 10 different values
			}
		}

		sortByMessageCount(sessions, true)

		// Verify sorted correctly
		for i := 1; i < len(sessions); i++ {
			assert.True(t, sessions[i-1].MessageCount <= sessions[i].MessageCount)
		}
	})
}

// TestListAllSessionsWithOptions_LargeDataset_Sorting tests end-to-end sorting with MongoDB
func TestListAllSessionsWithOptions_LargeDataset_Sorting(t *testing.T) {
	service, cleanup := setupTestStorage(t, nil)
	defer cleanup()
	require.NotNil(t, service)

	now := time.Now()
	size := 1000

	t.Logf("Creating %d test sessions with varied message counts...", size)

	// Create sessions with varied message counts
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < size; i++ {
		messageCount := rand.Intn(100) // 0-99 messages
		messages := make([]*session.Message, messageCount)
		for j := 0; j < messageCount; j++ {
			messages[j] = &session.Message{
				Content:   fmt.Sprintf("Message %d", j),
				Timestamp: now,
				Sender:    "user",
			}
		}

		sess := &session.Session{
			ID:          fmt.Sprintf("session-%d", i),
			UserID:      fmt.Sprintf("user-%d", i%50),
			Name:        fmt.Sprintf("Session %d", i),
			Messages:    messages,
			StartTime:   now.Add(-time.Duration(i) * time.Minute),
			TotalTokens: i * 10,
		}

		err := service.CreateSession(sess)
		require.NoError(t, err)
	}
	t.Logf("Created %d test sessions", size)

	// Test sorting by message count ascending
	t.Run("SortAscending", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     size,
			SortBy:    "message_count",
			SortOrder: "asc",
		}

		start := time.Now()
		result, err := service.ListAllSessionsWithOptions(opts)
		elapsed := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, size, len(result))

		// Verify ascending order
		for i := 1; i < len(result); i++ {
			assert.True(t, result[i-1].MessageCount <= result[i].MessageCount,
				"Not sorted ascending at index %d: %d > %d",
				i, result[i-1].MessageCount, result[i].MessageCount)
		}

		assert.True(t, elapsed < 2*time.Second,
			"Query took %v, expected < 2s", elapsed)
		t.Logf("Sorted %d sessions ascending in %v", size, elapsed)
	})

	// Test sorting by message count descending
	t.Run("SortDescending", func(t *testing.T) {
		opts := &SessionListOptions{
			Limit:     size,
			SortBy:    "message_count",
			SortOrder: "desc",
		}

		start := time.Now()
		result, err := service.ListAllSessionsWithOptions(opts)
		elapsed := time.Since(start)

		assert.NoError(t, err)
		assert.Equal(t, size, len(result))

		// Verify descending order
		for i := 1; i < len(result); i++ {
			assert.True(t, result[i-1].MessageCount >= result[i].MessageCount,
				"Not sorted descending at index %d: %d < %d",
				i, result[i-1].MessageCount, result[i].MessageCount)
		}

		assert.True(t, elapsed < 2*time.Second,
			"Query took %v, expected < 2s", elapsed)
		t.Logf("Sorted %d sessions descending in %v", size, elapsed)
	})

	// Test with pagination and sorting
	t.Run("PaginationWithSorting", func(t *testing.T) {
		pageSize := 100

		// Get first page
		opts := &SessionListOptions{
			Limit:     pageSize,
			Offset:    0,
			SortBy:    "message_count",
			SortOrder: "desc",
		}
		page1, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, pageSize, len(page1))

		// Get second page
		opts.Offset = pageSize
		page2, err := service.ListAllSessionsWithOptions(opts)
		assert.NoError(t, err)
		assert.Equal(t, pageSize, len(page2))

		// Verify pages are sorted correctly
		// Last item of page 1 should have >= message count than first item of page 2
		assert.True(t, page1[pageSize-1].MessageCount >= page2[0].MessageCount,
			"Pagination broke sorting: page1 last=%d, page2 first=%d",
			page1[pageSize-1].MessageCount, page2[0].MessageCount)
	})
}

// TestSortByMessageCount_StressTest tests with very large datasets
func TestSortByMessageCount_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	sizes := []int{10000, 50000, 100000}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size_%d", size), func(t *testing.T) {
			sessions := make([]*SessionMetadata, size)
			rand.Seed(time.Now().UnixNano())

			for i := 0; i < size; i++ {
				sessions[i] = &SessionMetadata{
					ID:           fmt.Sprintf("session-%d", i),
					MessageCount: rand.Intn(10000),
				}
			}

			start := time.Now()
			sortByMessageCount(sessions, false)
			elapsed := time.Since(start)

			// Verify correctness
			for i := 1; i < len(sessions); i++ {
				assert.True(t, sessions[i-1].MessageCount >= sessions[i].MessageCount,
					"Not sorted at index %d", i)
			}

			// Performance should scale as O(n log n)
			// For 100k items, should complete in a few seconds
			maxTime := time.Duration(size/1000) * time.Second
			if maxTime < time.Second {
				maxTime = time.Second
			}
			assert.True(t, elapsed < maxTime,
				"Sorting %d sessions took %v, expected < %v", size, elapsed, maxTime)

			t.Logf("Sorted %d sessions in %v (%.2f µs per item)",
				size, elapsed, float64(elapsed.Microseconds())/float64(size))
		})
	}
}
