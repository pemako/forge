package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pemako/mk/cache"
	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/example/internal/model"
	"github.com/pemako/mk/example/internal/repository"
	"github.com/pemako/mk/httpclient"
	"github.com/pemako/mk/lock"
	"github.com/pemako/mk/metric"
)

var (
	ErrUserNotFound   = mkerrors.Define(404, "USER_NOT_FOUND", "user not found")
	ErrEmailDuplicate = mkerrors.Define(409, "EMAIL_DUPLICATE", "email already exists")
)

type UserService struct {
	repo *repository.UserRepo
}

func NewUserService() *UserService {
	return &UserService{repo: &repository.UserRepo{}}
}

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
	return u, nil
}

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

func (s *UserService) List(ctx context.Context) ([]model.User, error) {
	return s.repo.List(ctx)
}

func (s *UserService) GetCached(ctx context.Context, id uint) (*model.User, error) {
	key := fmt.Sprintf("user:%d", id)

	// Try cache first
	c := cache.Client("default")
	if c != nil {
		if val, err := c.Get(ctx, key); err == nil {
			var u model.User
			if jsonErr := json.Unmarshal([]byte(val), &u); jsonErr == nil {
				return &u, nil
			}
		}
	}

	// Cache miss — query DB
	u, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Store in cache
	if c != nil {
		if data, jsonErr := json.Marshal(u); jsonErr == nil {
			_ = c.Set(ctx, key, string(data), 5*time.Minute)
		}
	}
	return u, nil
}

// GetExternalPost fetches a post from the JSONPlaceholder API, demonstrating
// the httpclient named-registry pattern.
func (s *UserService) GetExternalPost(ctx context.Context, id int) (map[string]any, error) {
	c := httpclient.Get("jsonplaceholder")
	if c == nil {
		return nil, mkerrors.ErrInternal
	}
	var post map[string]any
	err := c.Get(ctx, fmt.Sprintf("/posts/%d", id), &post)
	if err != nil {
		return nil, mkerrors.Wrap(mkerrors.ErrInternal, "fetch external post: %w", err)
	}
	return post, nil
}

// CreateWithLock creates a user guarded by a distributed Redis lock, so that
// two concurrent requests for the same email cannot both pass the uniqueness
// check at the same time. This demonstrates the lock.TryLock API.
func (s *UserService) CreateWithLock(ctx context.Context, name, email string) (*model.User, error) {
	locker := lock.New("localhost:6379") // in production: use configured Redis address
	lockKey := "user:email:" + email

	var result *model.User
	err := locker.TryLock(ctx, lockKey, 10*time.Second,
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
