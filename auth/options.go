// Package auth provides JWT-based authentication middleware and token utilities
// for gin HTTP servers. It is intentionally minimal: the framework handles
// signing, verification, and context storage; business logic defines its own
// Claims struct and secret management.
//
// Typical usage:
//
//	// main.go — register middleware
//	r.Use(auth.Required(
//	    auth.WithSecret(cfg.GetString("auth.secret")),
//	    auth.WithClaims(func() auth.Claims { return &UserClaims{} }),
//	))
//
//	// handler — extract claims
//	claims, ok := auth.FromContext[*UserClaims](c.Request.Context())
package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

// Claims is the interface that business-defined JWT claims structs must satisfy.
// Embed jwt.RegisteredClaims in your struct to implement it automatically.
type Claims interface {
	jwt.Claims
}

type options struct {
	secret     []byte
	keyFunc    jwt.Keyfunc
	expire     time.Duration
	issuer     string
	claimsFunc func() Claims // factory: returns a new empty claims instance for parsing
	log        core.Logger
}

// Option configures JWT middleware or token generation.
type Option func(*options)

// WithSecret sets the HMAC signing secret used for HS256 signing and verification.
func WithSecret(secret string) Option {
	return func(o *options) {
		o.secret = []byte(secret)
		o.keyFunc = func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return o.secret, nil
		}
	}
}

// WithKeyFunc sets a custom key function for token verification (e.g., multi-key rotation).
func WithKeyFunc(fn jwt.Keyfunc) Option {
	return func(o *options) { o.keyFunc = fn }
}

// WithExpire sets the token expiration duration used by Generate (default 24h).
func WithExpire(d time.Duration) Option {
	return func(o *options) { o.expire = d }
}

// WithIssuer sets the "iss" claim added to generated tokens.
func WithIssuer(issuer string) Option {
	return func(o *options) { o.issuer = issuer }
}

// WithClaims sets a factory function that returns a new empty claims instance.
// Required when parsing tokens into custom claims structs.
//
//	auth.WithClaims(func() auth.Claims { return &UserClaims{} })
func WithClaims(fn func() Claims) Option {
	return func(o *options) { o.claimsFunc = fn }
}

// WithLogger sets the logger for auth events.
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

func defaultOptions() options {
	return options{
		expire: 24 * time.Hour,
		log:    logger.Default(),
	}
}

// WithConfig reads auth settings from core.Config.
// Keys: auth.secret (HMAC signing secret), auth.expire (token duration, e.g. "24h"),
// auth.issuer (optional "iss" claim).
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("auth.secret"); v != "" {
			// reuse WithSecret logic
			o.secret = []byte(v)
			o.keyFunc = func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return o.secret, nil
			}
		}
		if v := cfg.GetDuration("auth.expire"); v > 0 {
			o.expire = v
		}
		if v := cfg.GetString("auth.issuer"); v != "" {
			o.issuer = v
		}
	}
}
