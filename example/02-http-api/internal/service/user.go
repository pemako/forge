package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/02-http-api/internal/model"
)

var (
	ErrUserNotFound  = errors.Define(404, "USER_NOT_FOUND", "user not found")
	ErrEmailConflict = errors.Define(409, "EMAIL_CONFLICT", "email already in use")
)

// UserService manages users in an in-memory store.
// In a real service this would use database.Manager.
type UserService struct {
	mu    sync.RWMutex
	store map[int]*model.User
	seq   atomic.Int64
}

func NewUserService() *UserService {
	return &UserService{store: make(map[int]*model.User)}
}

func (s *UserService) List(_ context.Context, offset, limit int) ([]*model.User, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]*model.User, 0, len(s.store))
	for _, u := range s.store {
		all = append(all, u)
	}
	total := int64(len(all))
	if offset >= len(all) {
		return []*model.User{}, total
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total
}

func (s *UserService) Get(_ context.Context, id int) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.store[id]
	if !ok {
		return nil, errors.Wrap(ErrUserNotFound, "id=%d", id)
	}
	return u, nil
}

func (s *UserService) Create(_ context.Context, name, email string) (*model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.store {
		if u.Email == email {
			return nil, errors.Wrap(ErrEmailConflict, "email=%s", email)
		}
	}
	id := int(s.seq.Add(1))
	u := &model.User{ID: id, Name: name, Email: email, CreatedAt: time.Now()}
	s.store[id] = u
	return u, nil
}

func (s *UserService) Update(_ context.Context, id int, name, email string) (*model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	u, ok := s.store[id]
	if !ok {
		return nil, errors.Wrap(ErrUserNotFound, "id=%d", id)
	}
	if name != "" {
		u.Name = name
	}
	if email != "" {
		u.Email = email
	}
	return u, nil
}

func (s *UserService) Delete(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.store[id]; !ok {
		return errors.Wrap(ErrUserNotFound, "id=%d", id)
	}
	delete(s.store, id)
	return nil
}
