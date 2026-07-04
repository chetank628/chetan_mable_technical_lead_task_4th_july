package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mable/mono/api/internal/auth"
	"github.com/mable/mono/api/internal/model"
	"github.com/mable/mono/api/internal/store"
)

const minPasswordLen = 8

// Signup creates an account, sets the session cookie, and returns the user.
// A duplicate email yields a generic 409 to avoid confirming which addresses
// are registered.
func (h *Handler) Signup(c *gin.Context) {
	creds, ok := bindCredentials(c)
	if !ok {
		return
	}

	hash, err := auth.HashPassword(creds.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	user, err := h.store.CreateUser(c.Request.Context(), creds.Email, hash)
	if err != nil {
		if errors.Is(err, store.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "could not create account"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		return
	}

	if err := h.auth.SetCookie(c, user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start session"})
		return
	}
	c.JSON(http.StatusCreated, userResponse(user))
}

// Login verifies credentials and sets the session cookie. Wrong email or
// password both return the same generic 401 (no user enumeration). A bcrypt
// comparison is run even for unknown users to blunt timing side-channels.
func (h *Handler) Login(c *gin.Context) {
	creds, ok := bindCredentials(c)
	if !ok {
		return
	}

	user, err := h.store.GetUserByEmail(c.Request.Context(), creds.Email)
	if err != nil {
		// Run a dummy comparison so response time does not reveal whether the
		// email exists.
		auth.CheckPassword("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv", creds.Password)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !auth.CheckPassword(user.PasswordHash, creds.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := h.auth.SetCookie(c, user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start session"})
		return
	}
	c.JSON(http.StatusOK, userResponse(user))
}

// Logout clears the session cookie.
func (h *Handler) Logout(c *gin.Context) {
	h.auth.ClearCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "logged out"})
}

// Me returns the currently authenticated user.
func (h *Handler) Me(c *gin.Context) {
	id, ok := auth.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, err := h.store.GetUserByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, userResponse(user))
}

// bindCredentials parses and validates the auth request body, writing the error
// response itself and returning ok=false on failure.
func bindCredentials(c *gin.Context) (model.Credentials, bool) {
	var creds model.Credentials
	if err := c.ShouldBindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "malformed JSON"})
		return model.Credentials{}, false
	}
	creds.Email = strings.ToLower(strings.TrimSpace(creds.Email))
	if creds.Email == "" || !strings.Contains(creds.Email, "@") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email required"})
		return model.Credentials{}, false
	}
	if len(creds.Password) < minPasswordLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return model.Credentials{}, false
	}
	return creds, true
}

func userResponse(u store.User) model.UserResponse {
	return model.UserResponse{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt}
}
