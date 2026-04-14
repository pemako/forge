// Package service implements the UserService gRPC server.
package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pemako/forge/auth"
	userv1 "github.com/pemako/forge/example/03-grpc-service/gen/user/v1"
)

// UserClaims is the JWT payload issued by Login.
type UserClaims struct {
	jwt.RegisteredClaims
	UserID int32  `json:"user_id"`
	Email  string `json:"email"`
}

// user is the in-memory domain entity.
type user struct {
	id        int32
	name      string
	email     string
	createdAt time.Time
}

func (u *user) proto() *userv1.User {
	return &userv1.User{
		Id:        u.id,
		Name:      u.name,
		Email:     u.email,
		CreatedAt: u.createdAt.Format(time.RFC3339),
	}
}

// UserService implements userv1.UserServiceServer.
type UserService struct {
	userv1.UnimplementedUserServiceServer

	authOpts []auth.Option
	mu       sync.RWMutex
	store    map[int32]*user
	seq      atomic.Int32
}

func NewUserService(authOpts []auth.Option) *UserService {
	return &UserService{
		authOpts: authOpts,
		store:    make(map[int32]*user),
	}
}

// requireAuth extracts JWT claims; returns Unauthenticated error if absent.
func (s *UserService) requireAuth(ctx context.Context) (*UserClaims, error) {
	claims, ok := auth.FromContext[*UserClaims](ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing or invalid token")
	}
	return claims, nil
}

// ── Login ─────────────────────────────────────────────────────────────────────

func (s *UserService) Login(_ context.Context, req *userv1.LoginRequest) (*userv1.LoginResponse, error) {
	if req.Email == "" || len(req.Password) < 6 {
		return nil, status.Error(codes.InvalidArgument, "email and password (min 6 chars) required")
	}
	// Demo: accept any well-formed credentials.
	claims := &UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{},
		UserID:           1,
		Email:            req.Email,
	}
	token, err := auth.Generate(claims, s.authOpts...)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}
	return &userv1.LoginResponse{Token: token}, nil
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

func (s *UserService) ListUsers(ctx context.Context, req *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	s.mu.RLock()
	all := make([]*user, 0, len(s.store))
	for _, u := range s.store {
		all = append(all, u)
	}
	s.mu.RUnlock()

	total := int64(len(all))
	offset := int((page - 1) * pageSize)
	if offset >= len(all) {
		return &userv1.ListUsersResponse{Total: total, Page: page, PageSize: pageSize}, nil
	}
	end := offset + int(pageSize)
	if end > len(all) {
		end = len(all)
	}
	protos := make([]*userv1.User, 0, end-offset)
	for _, u := range all[offset:end] {
		protos = append(protos, u.proto())
	}
	return &userv1.ListUsersResponse{Total: total, Page: page, PageSize: pageSize, Users: protos}, nil
}

// ── GetUser ───────────────────────────────────────────────────────────────────

func (s *UserService) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	u, ok := s.store[req.Id]
	s.mu.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	return &userv1.GetUserResponse{User: u.proto()}, nil
}

// ── CreateUser ────────────────────────────────────────────────────────────────

func (s *UserService) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	if req.Name == "" || req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "name and email are required")
	}
	s.mu.Lock()
	for _, u := range s.store {
		if u.email == req.Email {
			s.mu.Unlock()
			return nil, status.Errorf(codes.AlreadyExists, "email %s already in use", req.Email)
		}
	}
	id := s.seq.Add(1)
	u := &user{id: id, name: req.Name, email: req.Email, createdAt: time.Now()}
	s.store[id] = u
	s.mu.Unlock()
	return &userv1.CreateUserResponse{User: u.proto()}, nil
}

// ── UpdateUser ────────────────────────────────────────────────────────────────

func (s *UserService) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	u, ok := s.store[req.Id]
	if !ok {
		s.mu.Unlock()
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	if req.Name != "" {
		u.name = req.Name
	}
	if req.Email != "" {
		u.email = req.Email
	}
	s.mu.Unlock()
	return &userv1.UpdateUserResponse{User: u.proto()}, nil
}

// ── DeleteUser ────────────────────────────────────────────────────────────────

func (s *UserService) DeleteUser(ctx context.Context, req *userv1.DeleteUserRequest) (*userv1.DeleteUserResponse, error) {
	if _, err := s.requireAuth(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if _, ok := s.store[req.Id]; !ok {
		s.mu.Unlock()
		return nil, status.Errorf(codes.NotFound, "user %d not found", req.Id)
	}
	delete(s.store, req.Id)
	s.mu.Unlock()
	return &userv1.DeleteUserResponse{}, nil
}
