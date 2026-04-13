package idgen_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/mk/idgen"
)

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
	require.NoError(t, idgen.Init("order", idgen.WithWorkerID(5)))
	g := idgen.Get("order")
	require.NotNil(t, g)
	assert.Greater(t, g.Next(), int64(0))
}

func TestGet_notRegistered(t *testing.T) {
	assert.Nil(t, idgen.Get("no-such-generator"))
}

func TestPackageLevel_Next(t *testing.T) {
	assert.Greater(t, idgen.Next(), int64(0))
}

func TestPackageLevel_NextString(t *testing.T) {
	assert.NotEmpty(t, idgen.NextString())
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
