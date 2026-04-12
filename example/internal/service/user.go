package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pemako/mk/cache"
	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/example/internal/model"
	"github.com/pemako/mk/example/internal/repository"
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
