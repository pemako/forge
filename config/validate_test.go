package config_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pemako/forge/config"
	"github.com/pemako/forge/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingLogger records Warn calls so tests can assert on the unknown-key
// warning without touching any package-level logger global.
type capturingLogger struct {
	mu    sync.Mutex
	warns []string
	// fields holds the structured fields of the last Warn call.
	fields []core.Field
}

func (l *capturingLogger) Debug(context.Context, string, ...core.Field) {}
func (l *capturingLogger) Info(context.Context, string, ...core.Field)  {}
func (l *capturingLogger) Error(context.Context, string, ...core.Field) {}
func (l *capturingLogger) With(...core.Field) core.Logger               { return l }

func (l *capturingLogger) Warn(_ context.Context, msg string, fields ...core.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
	l.fields = fields
}

func (l *capturingLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warns)
}

// writeConfig writes a config file into a per-test temp dir and returns its path.
func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// --- (a) WithRequired -------------------------------------------------------

func TestWithRequired_allPresent(t *testing.T) {
	cfg, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithRequired("app.name", "server.http.port"),
	)
	require.NoError(t, err)
	assert.Equal(t, "test-service", cfg.GetString("app.name"))
}

func TestWithRequired_missingReturnsError(t *testing.T) {
	_, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithRequired("database.default.dsn"),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrMissingRequiredKey)
	assert.Contains(t, err.Error(), "database.default.dsn")
}

// TestWithRequired_namesEveryMissingKey is the core fail-fast guarantee: one
// restart must reveal every missing key, not just the first.
func TestWithRequired_namesEveryMissingKey(t *testing.T) {
	_, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithRequired(
			"database.default.dsn",
			"app.name", // present — must not be reported
			"cache.redis.addr",
			"idgen.worker_id",
		),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrMissingRequiredKey)

	msg := err.Error()
	assert.Contains(t, msg, "database.default.dsn")
	assert.Contains(t, msg, "cache.redis.addr")
	assert.Contains(t, msg, "idgen.worker_id")
	assert.NotContains(t, msg, "app.name")
}

// TestWithRequired_satisfiedByEnv proves the check runs against the merged
// config, so a key supplied only via the environment is not reported missing.
func TestWithRequired_satisfiedByEnv(t *testing.T) {
	t.Setenv("APP_DATABASE_DEFAULT_DSN", "file::memory:")
	_, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithEnv("APP"),
		config.WithRequired("database.default.dsn"),
	)
	require.NoError(t, err)
}

func TestWithRequired_noKeysIsNoop(t *testing.T) {
	_, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithRequired(),
	)
	require.NoError(t, err)
}

// TestWithRequired_withoutFileStillFailsFast covers the originally reported
// hole: Load with no file used to succeed and hand back zero values forever.
func TestWithRequired_withoutFileStillFailsFast(t *testing.T) {
	_, err := config.Load(config.WithRequired("app.name"))
	require.Error(t, err)
	assert.ErrorIs(t, err, config.ErrMissingRequiredKey)
	assert.Contains(t, err.Error(), "app.name")
}

// --- (b) unknown / typo'd keys ---------------------------------------------

func TestUnknownKeys_detectsTypo(t *testing.T) {
	path := writeConfig(t, "c.yaml", "app:\n  name: svc\nservre:\n  port: 8080\n")
	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	unknown := cfg.UnknownKeys([]string{"app", "server"})
	assert.Equal(t, []string{"servre.port"}, unknown)
}

func TestUnknownKeys_prefixCoversSubtree(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	// "server" must cover "server.http.port" and "server.http.timeout".
	assert.Empty(t, cfg.UnknownKeys([]string{"app", "server"}))
	// Exact-key declarations work too.
	assert.Empty(t, cfg.UnknownKeys([]string{"app.name", "app.env", "server"}))
}

func TestUnknownKeys_emptyDeclaredReportsAll(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	all := cfg.UnknownKeys(nil)
	assert.Contains(t, all, "app.name")
	assert.Contains(t, all, "server.http.port")
}

func TestUnknownKeys_sortedAndStable(t *testing.T) {
	path := writeConfig(t, "c.yaml", "zeta: 1\nalpha: 2\nmid: 3\n")
	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	assert.Equal(t, []string{"alpha", "mid", "zeta"}, cfg.UnknownKeys(nil))
}

func TestWithKnownKeys_warnsNotErrors(t *testing.T) {
	path := writeConfig(t, "c.yaml", "app:\n  name: svc\ndatabse:\n  dsn: x\n")
	log := &capturingLogger{}

	cfg, err := config.Load(
		config.WithFile(path),
		config.WithKnownKeys("app"),
		config.WithLogger(log),
	)
	// A typo must warn, never fail the load — existing callers carry extra keys.
	require.NoError(t, err)
	require.NotNil(t, cfg)

	require.Equal(t, 1, log.warnCount())
	var got []string
	for _, f := range log.fields {
		if f.Key == "unknown_keys" {
			got, _ = f.Value.([]string)
		}
	}
	assert.Equal(t, []string{"databse.dsn"}, got)
}

func TestWithKnownKeys_silentWhenAllDeclared(t *testing.T) {
	log := &capturingLogger{}
	_, err := config.Load(
		config.WithFile("testdata/config.yaml"),
		config.WithKnownKeys("app", "server"),
		config.WithLogger(log),
	)
	require.NoError(t, err)
	assert.Zero(t, log.warnCount())
}

// TestWithKnownKeys_notDeclaredNoWarning proves the detection is opt-in: a
// caller that never calls WithKnownKeys gets no warning at all.
func TestWithKnownKeys_notDeclaredNoWarning(t *testing.T) {
	path := writeConfig(t, "c.yaml", "whatever: 1\n")
	log := &capturingLogger{}
	_, err := config.Load(config.WithFile(path), config.WithLogger(log))
	require.NoError(t, err)
	assert.Zero(t, log.warnCount())
}

// TestWithKnownKeys_emptyDeclarationWarnsOnEverything distinguishes
// WithKnownKeys() from never calling it.
func TestWithKnownKeys_emptyDeclarationWarnsOnEverything(t *testing.T) {
	path := writeConfig(t, "c.yaml", "whatever: 1\n")
	log := &capturingLogger{}
	_, err := config.Load(
		config.WithFile(path),
		config.WithKnownKeys(),
		config.WithLogger(log),
	)
	require.NoError(t, err)
	assert.Equal(t, 1, log.warnCount())
}

// --- (c) IsSet --------------------------------------------------------------

func TestIsSet(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	assert.True(t, cfg.IsSet("app.name"))
	assert.True(t, cfg.IsSet("server.http.port"))
	assert.False(t, cfg.IsSet("database.default.dsn"))
	assert.False(t, cfg.IsSet("totally.absent"))
}

// TestIsSet_distinguishesZeroFromAbsent is the reason IsSet exists: GetInt
// returns 0 for both `port: 0` and a missing key.
func TestIsSet_distinguishesZeroFromAbsent(t *testing.T) {
	path := writeConfig(t, "c.yaml", "server:\n  port: 0\n")
	cfg, err := config.Load(config.WithFile(path))
	require.NoError(t, err)

	assert.Equal(t, 0, cfg.GetInt("server.port"))
	assert.Equal(t, 0, cfg.GetInt("server.missing"))
	assert.True(t, cfg.IsSet("server.port"), "explicit zero is set")
	assert.False(t, cfg.IsSet("server.missing"))
}

// TestIsSet_viaTypeAssertion documents the supported access path for callers
// that hold a core.Config (which does not declare IsSet).
func TestIsSet_viaTypeAssertion(t *testing.T) {
	loaded, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	var c core.Config = loaded
	s, ok := c.(interface{ IsSet(string) bool })
	require.True(t, ok, "concrete config must expose IsSet via assertion")
	assert.True(t, s.IsSet("app.name"))
	assert.False(t, s.IsSet("nope"))
}

// --- (e)/(f) Watch ----------------------------------------------------------

func TestWatch_errorsWhenWatchNotEnabled(t *testing.T) {
	cfg, err := config.Load(config.WithFile("testdata/config.yaml"))
	require.NoError(t, err)

	err = cfg.Watch("logger.level", func(any) {})
	require.Error(t, err, "Watch must not silently accept a callback it cannot fire")
	assert.ErrorIs(t, err, config.ErrWatchDisabled)
	assert.Contains(t, err.Error(), "logger.level")
}

func TestWatch_succeedsWhenEnabled(t *testing.T) {
	path := writeConfig(t, "c.yaml", "logger:\n  level: info\n")
	cfg, err := config.Load(config.WithFile(path), config.WithWatch())
	require.NoError(t, err)

	require.NoError(t, cfg.Watch("logger.level", func(any) {}))
}

// TestWatch_concurrentRegistration exercises the watchers map under -race.
// Before the mutex was added, concurrent Watch calls raced with each other and
// with the fsnotify callback's iteration of the same map.
func TestWatch_concurrentRegistration(t *testing.T) {
	path := writeConfig(t, "c.yaml", "logger:\n  level: info\napp:\n  name: x\n")
	cfg, err := config.Load(config.WithFile(path), config.WithWatch())
	require.NoError(t, err)

	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			key := "logger.level"
			if i%2 == 0 {
				key = "app.name"
			}
			assert.NoError(t, cfg.Watch(key, func(any) {}))
		}(i)
	}
	wg.Wait()
}

// TestWatch_concurrentWithFileChange drives real fsnotify callbacks while new
// watchers are registered, which is the exact interleaving that raced.
func TestWatch_concurrentWithFileChange(t *testing.T) {
	path := writeConfig(t, "c.yaml", "logger:\n  level: info\n")
	cfg, err := config.Load(config.WithFile(path), config.WithWatch())
	require.NoError(t, err)

	var fired sync.WaitGroup
	fired.Add(1)
	var once sync.Once
	require.NoError(t, cfg.Watch("logger.level", func(any) {
		once.Do(fired.Done)
	}))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 20 {
			_ = os.WriteFile(path,
				[]byte("logger:\n  level: debug\napp:\n  n: "+string(rune('a'+i%26))+"\n"), 0o600)
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			assert.NoError(t, cfg.Watch("app.n", func(any) {}))
		}
	}()
	wg.Wait()
}
