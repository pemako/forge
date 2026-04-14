package handler

import (
	"context"

	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/02-http-api/internal/model"
	"github.com/pemako/forge/example/02-http-api/internal/service"
	"github.com/pemako/forge/pagination"
)

// requireAuth extracts JWT claims from context.
// Returns 401 if auth.Optional did not store valid claims (missing/invalid token).
func requireAuth(ctx context.Context) (*UserClaims, error) {
	claims, ok := auth.FromContext[*UserClaims](ctx)
	if !ok {
		return nil, errors.Wrap(errors.ErrUnauthorized, "missing or invalid token")
	}
	return claims, nil
}

func listUsersHandler(svc *service.UserService) func(context.Context, *model.ListUsersInput) (*model.ListUsersOutput, error) {
	return func(ctx context.Context, input *model.ListUsersInput) (*model.ListUsersOutput, error) {
		if _, err := requireAuth(ctx); err != nil {
			return nil, err
		}
		p := pagination.FromParams(input.Page, input.PageSize)
		users, total := svc.List(ctx, pagination.Offset(p), p.PageSize)
		result := pagination.New(total, users, p)
		return &model.ListUsersOutput{Body: &result}, nil
	}
}

func getUserHandler(svc *service.UserService) func(context.Context, *model.GetUserInput) (*model.GetUserOutput, error) {
	return func(ctx context.Context, input *model.GetUserInput) (*model.GetUserOutput, error) {
		if _, err := requireAuth(ctx); err != nil {
			return nil, err
		}
		user, err := svc.Get(ctx, input.ID)
		if err != nil {
			return nil, err
		}
		return &model.GetUserOutput{Body: user}, nil
	}
}

func createUserHandler(svc *service.UserService) func(context.Context, *model.CreateUserInput) (*model.CreateUserOutput, error) {
	return func(ctx context.Context, input *model.CreateUserInput) (*model.CreateUserOutput, error) {
		if _, err := requireAuth(ctx); err != nil {
			return nil, err
		}
		user, err := svc.Create(ctx, input.Body.Name, input.Body.Email)
		if err != nil {
			return nil, err
		}
		return &model.CreateUserOutput{Body: user}, nil
	}
}

func updateUserHandler(svc *service.UserService) func(context.Context, *model.UpdateUserInput) (*model.UpdateUserOutput, error) {
	return func(ctx context.Context, input *model.UpdateUserInput) (*model.UpdateUserOutput, error) {
		if _, err := requireAuth(ctx); err != nil {
			return nil, err
		}
		user, err := svc.Update(ctx, input.ID, input.Body.Name, input.Body.Email)
		if err != nil {
			return nil, err
		}
		return &model.UpdateUserOutput{Body: user}, nil
	}
}

func deleteUserHandler(svc *service.UserService) func(context.Context, *model.DeleteUserInput) (*model.DeleteUserOutput, error) {
	return func(ctx context.Context, input *model.DeleteUserInput) (*model.DeleteUserOutput, error) {
		if _, err := requireAuth(ctx); err != nil {
			return nil, err
		}
		if err := svc.Delete(ctx, input.ID); err != nil {
			return nil, err
		}
		return &model.DeleteUserOutput{}, nil
	}
}
