// Package middleware holds the cross-cutting HTTP concerns. Right now that is
// authentication: every route except the health check requires a valid
// Supabase-issued token.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// contextKeyUserID is the key the authenticated user's id is stored under. A
// named type rather than a bare string so nothing else can collide with it.
const contextKeyUserID = "authenticated_user_id"

// supabaseAudience is the audience Supabase stamps on a signed-in user's
// token. Anonymous and service tokens carry different values, so checking it
// is what stops a service key being used as if it were a person.
const supabaseAudience = "authenticated"

// Authenticator verifies Supabase JWTs against the project's published JWKS.
//
// The keys are fetched over the network and cached, refreshing on their own
// schedule, so a key rotation on Supabase's side is picked up without a
// redeploy — and a token signed by a retired key stops verifying.
type Authenticator struct {
	keySet   jwk.Set
	issuer   string
	logger   *slog.Logger
	isJWTOff bool
}

// NewAuthenticator prepares JWT verification for a Supabase project URL such
// as https://abcdefg.supabase.co.
//
// Verification is asymmetric: the server holds only the public half, so a
// leaked deployment cannot mint tokens. That is why the JWT secret is never an
// input here.
func NewAuthenticator(
	ctx context.Context,
	supabaseProjectURL string,
	logger *slog.Logger,
) (*Authenticator, error) {
	trimmedURL := strings.TrimRight(strings.TrimSpace(supabaseProjectURL), "/")
	if trimmedURL == "" {
		return nil, fmt.Errorf("SUPABASE_URL is not set")
	}

	jwksURL := trimmedURL + "/auth/v1/.well-known/jwks.json"
	issuer := trimmedURL + "/auth/v1"

	cache, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("preparing the JWKS cache: %w", err)
	}
	if err := cache.Register(ctx, jwksURL,
		jwk.WithMinInterval(15*time.Minute)); err != nil {
		return nil, fmt.Errorf("registering the JWKS endpoint: %w", err)
	}

	// Fetch once up front so a misconfigured URL fails at startup rather than
	// on the first request a person makes.
	if _, err := cache.Lookup(ctx, jwksURL); err != nil {
		return nil, fmt.Errorf("fetching %s: %w", jwksURL, err)
	}

	cachedSet, err := cache.CachedSet(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("reading the cached JWKS: %w", err)
	}

	logger.Info("authentication ready", "jwks", jwksURL, "issuer", issuer)
	return &Authenticator{keySet: cachedSet, issuer: issuer, logger: logger}, nil
}

// NewDisabledAuthenticator returns an authenticator that lets every request
// through. It exists for local development against a database with no Supabase
// project attached, and refuses to be silent about it.
func NewDisabledAuthenticator(logger *slog.Logger) *Authenticator {
	logger.Warn("authentication is DISABLED; every request will be served unauthenticated")
	return &Authenticator{isJWTOff: true, logger: logger}
}

// RequireUser rejects any request without a valid, unexpired Supabase token.
func (authenticator *Authenticator) RequireUser() gin.HandlerFunc {
	return func(context *gin.Context) {
		if authenticator.isJWTOff {
			context.Next()
			return
		}

		rawToken, ok := bearerTokenFrom(context.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(context, "a bearer token is required")
			return
		}

		token, err := jwt.Parse([]byte(rawToken),
			jwt.WithKeySet(authenticator.keySet),
			jwt.WithValidate(true),
			jwt.WithIssuer(authenticator.issuer),
			jwt.WithAudience(supabaseAudience),
			// Tolerates small clock differences between Supabase and this
			// server without meaningfully extending a token's life.
			jwt.WithAcceptableSkew(30*time.Second),
		)
		if err != nil {
			// Logged at debug: a bad token is an ordinary event on a public
			// endpoint, not an operational fault worth alerting on.
			authenticator.logger.Debug("rejected a token", "error", err)
			abortUnauthorized(context, "your session is invalid or has expired")
			return
		}

		userID, hasSubject := token.Subject()
		if !hasSubject || userID == "" {
			abortUnauthorized(context, "your session is invalid or has expired")
			return
		}

		context.Set(contextKeyUserID, userID)
		context.Next()
	}
}

// UserIDFrom returns the authenticated user's id. Handlers do not currently
// scope data by user — this is a single-tenant app — but the id is carried so
// that adding a second user later is a query change rather than a redesign.
func UserIDFrom(context *gin.Context) (string, bool) {
	value, exists := context.Get(contextKeyUserID)
	if !exists {
		return "", false
	}
	userID, isString := value.(string)
	return userID, isString
}

// bearerTokenFrom pulls the credential out of an Authorization header,
// accepting the scheme case-insensitively as RFC 7235 requires.
func bearerTokenFrom(headerValue string) (string, bool) {
	const bearerPrefix = "bearer "
	if len(headerValue) <= len(bearerPrefix) {
		return "", false
	}
	if !strings.EqualFold(headerValue[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(headerValue[len(bearerPrefix):])
	return token, token != ""
}

func abortUnauthorized(context *gin.Context, message string) {
	context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
