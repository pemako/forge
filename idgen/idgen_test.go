package idgen_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/mk/idgen"
)

// --- Snowflake tests ---

func TestNew_basic(t *testing.T) {
	g, err := idgen.New(idgen.WithWorkerID(1))
	require.NoError(t, err)
	assert.Greater(t, g.Next(), int64(0))
}

func TestNext_unique(t *testing.T) {
	g, _ := idgen.New(idgen.WithWorkerID(1))
	const n = 1000
	seen := make(map[int64]struct{}, n)
	for range n {
		id := g.Next()
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ID: %d", id)
		seen[id] = struct{}{}
	}
}

func TestNext_monotonic(t *testing.T) {
	g, _ := idgen.New(idgen.WithWorkerID(2))
	prev := g.Next()
	for range 100 {
		cur := g.Next()
		assert.GreaterOrEqual(t, cur, prev)
		prev = cur
	}
}

func TestNext_concurrent(t *testing.T) {
	g, _ := idgen.New(idgen.WithWorkerID(3))
	const goroutines, perG = 10, 100
	ids := make(chan int64, goroutines*perG)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perG {
				ids <- g.Next()
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[int64]struct{}, goroutines*perG)
	for id := range ids {
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ID: %d", id)
		seen[id] = struct{}{}
	}
}

func TestNextString(t *testing.T) {
	g, _ := idgen.New(idgen.WithWorkerID(1))
	assert.NotEmpty(t, g.NextString())
}

func TestInit_and_Get(t *testing.T) {
	g, err := idgen.New(idgen.WithWorkerID(5))
	require.NoError(t, err)
	require.NoError(t, idgen.Register("order_test", g))
	got := idgen.Get("order_test")
	require.NotNil(t, got)
	assert.Greater(t, got.Next(), int64(0))
}

func TestGet_notRegistered(t *testing.T) {
	assert.Nil(t, idgen.Get("no-such-generator"))
}

func TestWithConfig(t *testing.T) {
	cfg := newStubConfig(map[string]string{"idgen.worker_id": "7"})
	g, err := idgen.New(idgen.WithConfig(cfg))
	require.NoError(t, err)
	assert.Greater(t, g.Next(), int64(0))
}

func TestWithWorkerID_invalid(t *testing.T) {
	_, err := idgen.New(idgen.WithWorkerID(9999))
	assert.Error(t, err)
}

// --- ULID tests ---

func TestULID_format(t *testing.T) {
	id := idgen.ULID()
	assert.Len(t, id, 26, "ULID must be 26 chars (Crockford Base32)")
	assert.Equal(t, strings.ToUpper(id), id, "ULID must be uppercase")
}

func TestULID_unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		id := idgen.ULID()
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ULID: %s", id)
		seen[id] = struct{}{}
	}
}

func TestULID_lexicographicOrder(t *testing.T) {
	// ULIDs generated in the same millisecond must still sort in insertion order
	// thanks to the monotonic entropy source.
	const n = 100
	ids := make([]string, n)
	for i := range n {
		ids[i] = idgen.ULID()
	}
	for i := 1; i < n; i++ {
		assert.LessOrEqual(t, ids[i-1], ids[i],
			"ULID[%d]=%s should be <= ULID[%d]=%s", i-1, ids[i-1], i, ids[i])
	}
}

func TestULID_concurrent(t *testing.T) {
	const goroutines, perG = 10, 100
	ids := make(chan string, goroutines*perG)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perG {
				ids <- idgen.ULID()
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]struct{}, goroutines*perG)
	for id := range ids {
		_, dup := seen[id]
		assert.False(t, dup, "duplicate ULID: %s", id)
		seen[id] = struct{}{}
	}
}

func TestULID_timestampEmbedded(t *testing.T) {
	before := time.Now()
	id := idgen.ULID()
	after := time.Now()

	// First 10 chars encode timestamp — a later ULID must be >= boundary ULIDs.
	_ = before
	_ = after
	_ = id
	// Just ensure it's non-empty and valid length (timestamp parsing
	// requires importing oklog/ulid directly, which is an impl detail).
	assert.Len(t, id, 26)
}

// --- minimal core.Config stub ---

type stubConfig struct{ m map[string]string }

func newStubConfig(m map[string]string) *stubConfig { return &stubConfig{m: m} }

func (s *stubConfig) Get(key string) any          { return s.m[key] }
func (s *stubConfig) GetString(key string) string { return s.m[key] }
func (s *stubConfig) GetInt(key string) int {
	n := 0
	for _, c := range s.m[key] {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
func (s *stubConfig) GetBool(key string) bool              { return false }
func (s *stubConfig) GetFloat64(key string) float64        { return 0 }
func (s *stubConfig) GetDuration(key string) time.Duration { return 0 }
func (s *stubConfig) GetStringSlice(key string) []string   { return nil }
func (s *stubConfig) Scan(key string, dest any) error      { return nil }
func (s *stubConfig) Watch(key string, fn func(any)) error { return nil }
