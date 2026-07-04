// Package auth implements password hashing, JWT issuance/verification, and the
// HttpOnly-cookie session model plus a gin middleware that gates protected
// routes. The JWT is never exposed to JavaScript: it lives only in a
// Secure/HttpOnly cookie to defend against XSS token theft.
package auth

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// CookieName is the name of the session cookie carrying the JWT.
const CookieName = "mable_session"

// contextUserKey is the gin context key under which the authenticated user id
// is stored by RequireAuth.
const contextUserKey = "auth_user_id"

// ErrInvalidToken is returned when a token is missing, malformed, or expired.
var ErrInvalidToken = errors.New("invalid or expired token")

// Authenticator issues and verifies tokens and writes/clears the session
// cookie. It is configured once at boot from config.
type Authenticator struct {
	secret   []byte
	ttl      time.Duration
	secure   bool
	sameSite http.SameSite
}

// New builds an Authenticator. secure and sameSite are derived from the
// deployment environment: prod uses Secure + SameSite=None for cross-site SPA
// requests; local dev uses non-Secure + SameSite=Lax.
func New(secret []byte, ttl time.Duration, prod bool) *Authenticator {
	sameSite := http.SameSiteLaxMode
	if prod {
		sameSite = http.SameSiteNoneMode
	}
	return &Authenticator{secret: secret, ttl: ttl, secure: prod, sameSite: sameSite}
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether password matches the stored bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// claims is the JWT payload. Subject carries the user id as a string.
type claims struct {
	jwt.RegisteredClaims
}

// issue mints a signed JWT for the given user id.
func (a *Authenticator) issue(userID int64) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(a.ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return tok.SignedString(a.secret)
}

// verify parses and validates a token string, returning the user id.
func (a *Authenticator) verify(tokenStr string) (int64, error) {
	c := &claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return a.secret, nil
	})
	if err != nil || !tok.Valid {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, ErrInvalidToken
	}
	return id, nil
}

// SetCookie issues a token for userID and writes it as the session cookie.
func (a *Authenticator) SetCookie(c *gin.Context, userID int64) error {
	token, err := a.issue(userID)
	if err != nil {
		return err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(a.ttl.Seconds()),
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: a.sameSite,
	})
	return nil
}

// ClearCookie expires the session cookie (logout).
func (a *Authenticator) ClearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.secure,
		SameSite: a.sameSite,
	})
}

// RequireAuth is gin middleware that rejects requests lacking a valid session
// cookie with 401 and otherwise stashes the user id in the request context.
func (a *Authenticator) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(CookieName)
		if err != nil || cookie == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		userID, err := a.verify(cookie)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set(contextUserKey, userID)
		c.Next()
	}
}

// UserID returns the authenticated user id stashed by RequireAuth.
func UserID(c *gin.Context) (int64, bool) {
	v, ok := c.Get(contextUserKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}
