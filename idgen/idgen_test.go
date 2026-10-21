package idgen_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/forge/idgen"
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
		wg.Go(func() {
			for range perG {
				ids <- g.Next()
			}
		})
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

// --- UUID tests ---

func TestUUID_format(t *testing.T) {
	id := idgen.UUID()
	assert.Len(t, id, 36, "UUID must be 36 chars (8-4-4-4-12)")
	assert.Equal(t, strings.ToLower(id), id, "UUID must be lowercase")
	assert.Equal(t, byte('7'), id[14], "UUID must be version 7")
}

func TestUUID_unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		id := idgen.UUID()
		_, dup := seen[id]
		assert.False(t, dup, "duplicate UUID: %s", id)
		seen[id] = struct{}{}
	}
}

func TestUUID_lexicographicOrder(t *testing.T) {
	// Version 7 UUIDs generated in the same millisecond must still sort in
	// insertion order — NewV7 guarantees strictly increasing values.
	const n = 100
	ids := make([]string, n)
	for i := range n {
		ids[i] = idgen.UUID()
	}
	for i := 1; i < n; i++ {
		assert.Less(t, ids[i-1], ids[i],
			"UUID[%d]=%s should be < UUID[%d]=%s", i-1, ids[i-1], i, ids[i])
	}
}

func TestUUID_concurrent(t *testing.T) {
	const goroutines, perG = 10, 100
	ids := make(chan string, goroutines*perG)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for range perG {
				ids <- idgen.UUID()
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]struct{}, goroutines*perG)
	for id := range ids {
		_, dup := seen[id]
		assert.False(t, dup, "duplicate UUID: %s", id)
		seen[id] = struct{}{}
	}
}

func TestUUIDv4_format(t *testing.T) {
	id := idgen.UUIDv4()
	assert.Len(t, id, 36, "UUIDv4 must be 36 chars (8-4-4-4-12)")
	assert.Equal(t, byte('4'), id[14], "UUIDv4 must be version 4")
	assert.NotEqual(t, id, idgen.UUIDv4(), "UUIDv4 must not repeat")
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

func TestNew_WithRequireExplicitWorkerID_DerivedFails(t *testing.T) {
	// No WithWorkerID → derived from IP → WithRequireExplicitWorkerID must reject it.
	_, err := idgen.New(idgen.WithRequireExplicitWorkerID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker_id must be set explicitly")
}

func TestNew_WithRequireExplicitWorkerID_ExplicitPasses(t *testing.T) {
	// Explicit WorkerID + require flag → must succeed.
	g, err := idgen.New(
		idgen.WithWorkerID(42),
		idgen.WithRequireExplicitWorkerID(),
	)
	require.NoError(t, err)
	assert.Greater(t, g.Next(), int64(0))
}

func TestNew_WithRequireExplicitWorkerID_ConfigPasses(t *testing.T) {
	// worker_id set via config → derived=false → must succeed even with require flag.
	cfg := newStubConfig(map[string]string{"idgen.worker_id": "7"})
	g, err := idgen.New(
		idgen.WithConfig(cfg),
		idgen.WithRequireExplicitWorkerID(),
	)
	require.NoError(t, err)
	assert.Greater(t, g.Next(), int64(0))
}
