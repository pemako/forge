package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pemako/forge/cache"
	"github.com/pemako/forge/circuitbreaker"
	mkerrors "github.com/pemako/forge/errors"
	"github.com/pemako/forge/event"
	"github.com/pemako/forge/httpclient"
	"github.com/pemako/forge/lock"
	"github.com/pemako/forge/metric"

	"github.com/pemako/forge/example/full/internal/model"
	"github.com/pemako/forge/example/full/internal/repository"
)

var (
	ErrUserNotFound   = mkerrors.Define(404, "USER_NOT_FOUND", "user not found")
	ErrEmailDuplicate = mkerrors.Define(409, "EMAIL_DUPLICATE", "email already exists")
)

type UserService struct {
	repo   *repository.UserRepo
	bus    *event.Bus
	cb     *circuitbreaker.CircuitBreaker
	locker *lock.Locker
}

// NewUserService creates a UserService wired with event bus, circuit breaker,
// and distributed locker (all demonstrated in this example service).
func NewUserService(bus *event.Bus, cb *circuitbreaker.CircuitBreaker, locker *lock.Locker) *UserService {
	return &UserService{
		repo:   &repository.UserRepo{},
		bus:    bus,
		cb:     cb,
		locker: locker,
	}
}

// Create persists a new user, publishes to the in-process event bus (event
// package) and the cross-instance Redis Stream (stream package), and records
// Prometheus/OTLP metrics.
func (s *UserService) Create(ctx context.Context, name, email string) (*model.User, error) {
	end := metric.StartTimer(ctx, "user_create")
	defer end()

	u := &model.User{Name: name, Email: email}
	if err := s.repo.Create(ctx, u); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Duplicate") {
			return nil, ErrEmailDuplicate
		}
		return nil, mkerrors.Wrap(mkerrors.ErrInternal, "create user: %w", err)
	}
	metric.Count(ctx, "user_created_total", 1)

	// Publish to in-process event bus (event package).
	_ = s.bus.Publish(ctx, "user.created", u)

	// stream.Publish is intentionally omitted here — streaming is optional
	// and wired at the main() level via the Bus instance.

	return u, nil
}

// Get fetches a single user by primary key.
func (s *UserService) Get(ctx context.Context, id uint) (*model.User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, mkerrors.Wrap(mkerrors.ErrInternal, "get user: %w", err)
	}
	if u == nil {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// List returns a page of users for the given offset/limit (pagination package
// converts page/page_size to these values in the handler layer).
func (s *UserService) List(ctx context.Context, offset, limit int) ([]model.User, error) {
	return s.repo.List(ctx, offset, limit)
}

// Count returns the total number of users (used by pagination.New in the handler).
func (s *UserService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

// GetCached demonstrates the cache-aside pattern using the cache package.
// It tries the memory cache first; on a miss it queries the DB and back-fills.
func (s *UserService) GetCached(ctx context.Context, id uint) (*model.User, error) {
	key := fmt.Sprintf("user:%d", id)

	// Try cache first (cache package — memory backend "default").
	c := cache.Client("default")
	if c != nil {
		if val, err := c.Get(ctx, key); err == nil {
			var u model.User
			if jsonErr := json.Unmarshal([]byte(val), &u); jsonErr == nil {
				return &u, nil
			}
		}
	}

	// Cache miss — query DB.
	u, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Back-fill cache with a 5-minute TTL.
	if c != nil {
		if data, jsonErr := json.Marshal(u); jsonErr == nil {
			_ = c.Set(ctx, key, string(data), 5*time.Minute)
		}
	}
	return u, nil
}

// GetExternalPost fetches a post from the JSONPlaceholder API, demonstrating
// the httpclient named-registry pattern wrapped in the circuit breaker.
func (s *UserService) GetExternalPost(ctx context.Context, id int) (map[string]any, error) {
	c := httpclient.Get("jsonplaceholder")
	if c == nil {
		return nil, mkerrors.ErrInternal
	}
	var post map[string]any
	err := s.cb.Execute(ctx, func(ctx context.Context) error {
		return c.Get(ctx, fmt.Sprintf("/posts/%d", id), &post)
	})
	if errors.Is(err, circuitbreaker.ErrOpen) {
		return nil, mkerrors.Define(503, "SERVICE_UNAVAILABLE", "external service temporarily unavailable")
	}
	if err != nil {
		return nil, mkerrors.Wrap(mkerrors.ErrInternal, "fetch external post: %w", err)
	}
	return post, nil
}

// CreateWithLock creates a user guarded by a distributed Redis lock, ensuring
// that concurrent requests for the same email cannot both pass the uniqueness
// check simultaneously. Demonstrates the lock package TryLock API.
func (s *UserService) CreateWithLock(ctx context.Context, name, email string) (*model.User, error) {
	lockKey := "user:email:" + email

	var result *model.User
	err := s.locker.TryLock(ctx, lockKey, 10*time.Second,
		func(lockCtx context.Context) error {
			u, err := s.Create(lockCtx, name, email)
			if err != nil {
				return err
			}
			result = u
			return nil
		},
		lock.WithWait(3*time.Second, 100*time.Millisecond),
	)
	if errors.Is(err, lock.ErrLockNotAcquired) {
		return nil, mkerrors.Define(409, "DUPLICATE_REQUEST", "concurrent request in progress")
	}
	return result, err
}
