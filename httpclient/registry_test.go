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

func TestRegister_DuplicateError(t *testing.T) {
	reset()
	Register("dup", WithBaseURL("http://a.test"))
	err := Register("dup", WithBaseURL("http://b.test"))
	require.ErrorIs(t, err, ErrAlreadyRegistered)
	// Original client still there
	require.Equal(t, "http://a.test", Get("dup").opts.baseURL)
}

func TestReplace_Succeeds(t *testing.T) {
	reset()
	Register("r", WithBaseURL("http://old.test"))
	require.NoError(t, Replace("r", WithBaseURL("http://new.test")))
	require.Equal(t, "http://new.test", Get("r").opts.baseURL)
}

func TestCloseAll_ClearsRegistry(t *testing.T) {
	reset()
	Register("a", WithBaseURL("http://a.test"))
	Register("b", WithBaseURL("http://b.test"))
	CloseAll()
	require.Nil(t, Get("a"))
	require.Nil(t, Get("b"))
}
