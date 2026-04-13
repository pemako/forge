package idgen

import (
	"crypto/rand"
	"sync"

	"github.com/oklog/ulid/v2"
)

// ULID returns a new Universally Unique Lexicographically Sortable Identifier.
// ULIDs are 128-bit values encoded as 26-character Crockford Base32 strings.
// The first 48 bits encode the current millisecond timestamp, making them
// lexicographically sortable by creation time — useful for object storage keys,
// log correlation IDs, and any scenario where string-sortable IDs are needed.
//
// ULIDs are generated using a monotonic entropy source within the same
// millisecond to guarantee strict ordering even under high throughput.
//
// Example output: "01ARZ3NDEKTSV4RRFFQ69G5FAV".
func ULID() string {
	return defaultULIDGenerator.next()
}

// --- package-level ULID generator ---

var defaultULIDGenerator = newULIDGen()

type ulidGen struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

func newULIDGen() *ulidGen {
	return &ulidGen{
		entropy: ulid.Monotonic(rand.Reader, 0),
	}
}

func (g *ulidGen) next() string {
	g.mu.Lock()
	id := ulid.MustNew(ulid.Now(), g.entropy)
	g.mu.Unlock()
	return id.String()
}
