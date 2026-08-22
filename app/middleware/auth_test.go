package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// Supabase signs with ES256, so the tests use the same algorithm rather than a
// more convenient one — a verifier that only works for HMAC would pass a test
// suite built on HMAC and fail in production.
func newSigningKey(t *testing.T) (jwk.Key, jwk.Key) {
	t.Helper()

	rawKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	privateKey, err := jwk.Import(rawKey)
	if err != nil {
		t.Fatalf("importing the private key: %v", err)
	}
	if err := privateKey.Set(jwk.KeyIDKey, "test-key"); err != nil {
		t.Fatalf("setting the key id: %v", err)
	}
	if err := privateKey.Set(jwk.AlgorithmKey, jwa.ES256()); err != nil {
		t.Fatalf("setting the algorithm: %v", err)
	}
	publicKey, err := jwk.PublicKeyOf(privateKey)
	if err != nil {
		t.Fatalf("deriving the public key: %v", err)
	}
	return privateKey, publicKey
}

// startJWKSServer stands in for a Supabase project, publishing the public half
// of the signing key at the path the authenticator looks for.
func startJWKSServer(t *testing.T, publicKey jwk.Key) *httptest.Server {
	t.Helper()

	keySet := jwk.NewSet()
	if err := keySet.AddKey(publicKey); err != nil {
		t.Fatalf("building the key set: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/auth/v1/.well-known/jwks.json" {
				writer.WriteHeader(http.StatusNotFound)
				return
			}
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(keySet)
		}))
	t.Cleanup(server.Close)
	return server
}

type tokenClaims struct {
	issuer    string
	audience  string
	subject   string
	expiresAt time.Time
}

func signToken(t *testing.T, signingKey jwk.Key, claims tokenClaims) string {
	t.Helper()

	builder := jwt.NewBuilder().
		Issuer(claims.issuer).
		Audience([]string{claims.audience}).
		Subject(claims.subject).
		IssuedAt(time.Now()).
		Expiration(claims.expiresAt)

	token, err := builder.Build()
	if err != nil {
		t.Fatalf("building the token: %v", err)
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), signingKey))
	if err != nil {
		t.Fatalf("signing the token: %v", err)
	}
	return string(signed)
}

// requestWith runs one request through the middleware and reports the status
// and the user id the handler saw.
func requestWith(t *testing.T, authenticator *Authenticator, authorizationHeader string) (int, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()

	var observedUserID string
	engine.GET("/private", authenticator.RequireUser(), func(context *gin.Context) {
		observedUserID, _ = UserIDFrom(context)
		context.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	if authorizationHeader != "" {
		request.Header.Set("Authorization", authorizationHeader)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	_, _ = io.Copy(io.Discard, recorder.Body)
	return recorder.Code, observedUserID
}

func TestRequireUser(t *testing.T) {
	signingKey, publicKey := newSigningKey(t)
	jwksServer := startJWKSServer(t, publicKey)

	logger := slog.New(slog.DiscardHandler)
	authenticator, err := NewAuthenticator(context.Background(), jwksServer.URL, logger)
	if err != nil {
		t.Fatalf("building the authenticator: %v", err)
	}

	validClaims := tokenClaims{
		issuer:    jwksServer.URL + "/auth/v1",
		audience:  supabaseAudience,
		subject:   "user-123",
		expiresAt: time.Now().Add(time.Hour),
	}

	t.Run("a valid token is accepted and carries the user id", func(t *testing.T) {
		status, userID := requestWith(t, authenticator,
			"Bearer "+signToken(t, signingKey, validClaims))
		if status != http.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
		if userID != "user-123" {
			t.Errorf("user id = %q, want %q", userID, "user-123")
		}
	})

	// RFC 7235 makes the scheme case-insensitive, and clients do vary.
	t.Run("the bearer scheme is case-insensitive", func(t *testing.T) {
		status, _ := requestWith(t, authenticator,
			"bearer "+signToken(t, signingKey, validClaims))
		if status != http.StatusOK {
			t.Errorf("status = %d, want 200", status)
		}
	})

	rejectionCases := []struct {
		name   string
		header string
	}{
		{"no header at all", ""},
		{"the scheme without a token", "Bearer "},
		{"a token with no scheme", signToken(t, signingKey, validClaims)},
		{"a wholly malformed token", "Bearer not-a-jwt"},
	}
	for _, testCase := range rejectionCases {
		t.Run(testCase.name+" is rejected", func(t *testing.T) {
			status, _ := requestWith(t, authenticator, testCase.header)
			if status != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", status)
			}
		})
	}

	t.Run("an expired token is rejected", func(t *testing.T) {
		expired := validClaims
		expired.expiresAt = time.Now().Add(-time.Hour)
		status, _ := requestWith(t, authenticator,
			"Bearer "+signToken(t, signingKey, expired))
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	// A token minted by some other Supabase project must not open this one.
	t.Run("a token from another issuer is rejected", func(t *testing.T) {
		wrongIssuer := validClaims
		wrongIssuer.issuer = "https://someone-else.supabase.co/auth/v1"
		status, _ := requestWith(t, authenticator,
			"Bearer "+signToken(t, signingKey, wrongIssuer))
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	// Supabase stamps a different audience on service and anonymous tokens;
	// only a signed-in person's token should pass.
	t.Run("a token for another audience is rejected", func(t *testing.T) {
		wrongAudience := validClaims
		wrongAudience.audience = "service_role"
		status, _ := requestWith(t, authenticator,
			"Bearer "+signToken(t, signingKey, wrongAudience))
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})

	// The signature is the whole point: a well-formed token with every claim
	// correct must still fail if it was signed by a key we do not trust.
	t.Run("a token signed by an untrusted key is rejected", func(t *testing.T) {
		attackerKey, _ := newSigningKey(t)
		status, _ := requestWith(t, authenticator,
			"Bearer "+signToken(t, attackerKey, validClaims))
		if status != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", status)
		}
	})
}

func TestNewAuthenticatorRejectsAnEmptyURL(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	if _, err := NewAuthenticator(context.Background(), "  ", logger); err == nil {
		t.Error("an empty SUPABASE_URL should be an error, not a server that trusts everything")
	}
}

func TestDisabledAuthenticatorLetsRequestsThrough(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	status, _ := requestWith(t, NewDisabledAuthenticator(logger), "")
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
}
