// httpclient/registry_test.go
// White-box tests for registry behaviour (needs access to private fields).
package httpclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// reset clears the registry. For use in tests only.
func reset() {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
}

func TestRegister_AcceptsInstance(t *testing.T) {
	reset()
	c := New(WithBaseURL("http://a.test"))
	require.NoError(t, Register("a", c))
	require.Equal(t, c, Get("a"))
}

func TestRegister_DuplicateError(t *testing.T) {
	reset()
	c := New(WithBaseURL("http://a.test"))
	Register("dup", c)
	err := Register("dup", New(WithBaseURL("http://b.test")))
	require.ErrorIs(t, err, ErrAlreadyRegistered)
	require.Equal(t, c, Get("dup")) // original preserved
}

func TestReplace_AcceptsInstance(t *testing.T) {
	reset()
	old := New(WithBaseURL("http://old.test"))
	Register("r", old)
	newC := New(WithBaseURL("http://new.test"))
	require.NoError(t, Replace("r", newC))
	require.Equal(t, newC, Get("r"))
}

func TestCloseAll_ReturnsNil(t *testing.T) {
	reset()
	Register("a", New(WithBaseURL("http://a.test")))
	require.NoError(t, CloseAll())
	require.Nil(t, Get("a"))
}
