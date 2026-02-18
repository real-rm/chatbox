package storage

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// BenchmarkSortByMessageCount benchmarks the sorting performance with different dataset sizes
// This measures the O(n log n) sort.Slice implementation that replaced the O(n²) bubble sort
func BenchmarkSortByMessageCount(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		// Create test data with random message counts to simulate real-world scenarios
		sessions := make([]*SessionMetadata, size)
		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				UserID:       "user-1",
				StartTime:    time.Now().Add(-time.Duration(i) * time.Hour),
				MessageCount: rand.Intn(1000), // Random message counts
				TotalTokens:  i * 10,
			}
		}

		b.Run(fmt.Sprintf("Ascending_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Create a copy for each iteration to avoid sorting already sorted data
				sessionsCopy := make([]*SessionMetadata, len(sessions))
				copy(sessionsCopy, sessions)
				sortByMessageCount(sessionsCopy, true)
			}
		})

		b.Run(fmt.Sprintf("Descending_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Create a copy for each iteration
				sessionsCopy := make([]*SessionMetadata, len(sessions))
				copy(sessionsCopy, sessions)
				sortByMessageCount(sessionsCopy, false)
			}
		})
	}
}

// BenchmarkSortByMessageCount_WorstCase benchmarks the worst-case scenario (reverse sorted)
func BenchmarkSortByMessageCount_WorstCase(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		// Create reverse-sorted data (worst case for many algorithms)
		sessions := make([]*SessionMetadata, size)
		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				UserID:       "user-1",
				StartTime:    time.Now().Add(-time.Duration(i) * time.Hour),
				MessageCount: size - i, // Reverse sorted
				TotalTokens:  i * 10,
			}
		}

		b.Run(fmt.Sprintf("ReverseSorted_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sessionsCopy := make([]*SessionMetadata, len(sessions))
				copy(sessionsCopy, sessions)
				sortByMessageCount(sessionsCopy, true)
			}
		})
	}
}

// BenchmarkSortByMessageCount_BestCase benchmarks the best-case scenario (already sorted)
func BenchmarkSortByMessageCount_BestCase(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		// Create already-sorted data (best case)
		sessions := make([]*SessionMetadata, size)
		for i := 0; i < size; i++ {
			sessions[i] = &SessionMetadata{
				ID:           fmt.Sprintf("session-%d", i),
				UserID:       "user-1",
				StartTime:    time.Now().Add(-time.Duration(i) * time.Hour),
				MessageCount: i, // Already sorted
				TotalTokens:  i * 10,
			}
		}

		b.Run(fmt.Sprintf("AlreadySorted_%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sessionsCopy := make([]*SessionMetadata, len(sessions))
				copy(sessionsCopy, sessions)
				sortByMessageCount(sessionsCopy, true)
			}
		})
	}
}
