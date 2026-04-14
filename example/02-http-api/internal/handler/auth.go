package handler

import (
	"context"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/02-http-api/internal/model"
)

func loginHandler(authOpts []auth.Option) func(context.Context, *model.LoginInput) (*model.LoginOutput, error) {
	return func(_ context.Context, input *model.LoginInput) (*model.LoginOutput, error) {
		// Demo: accept any well-formed email + password.
		// In production: look up user by email and verify hashed password.
		claims := &UserClaims{
			RegisteredClaims: jwt.RegisteredClaims{},
			UserID:           1,
			Email:            input.Body.Email,
		}
		token, err := auth.Generate(claims, authOpts...)
		if err != nil {
			return nil, errors.Wrap(errors.ErrInternal, "generate token: %v", err)
		}
		out := &model.LoginOutput{}
		out.Body.Token = token
		return out, nil
	}
}
