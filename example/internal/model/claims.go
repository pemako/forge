package model

import "github.com/golang-jwt/jwt/v5"

// UserClaims is the JWT claims struct used by this service.
// Embedding jwt.RegisteredClaims satisfies the auth.Claims interface.
type UserClaims struct {
	jwt.RegisteredClaims
	UserID int64  `json:"uid"`
	Role   string `json:"role"`
}
