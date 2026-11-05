package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultIterations  = 50000
	sessionTTL         = 24 * time.Hour
	authConfigFileName = "server-auth.json"
	secretFileName     = ".server_secret"
	sessionCookieName  = "lunaris_admin_session"
)

// AuthConfig holds encrypted and hashed credential data on disk.
type AuthConfig struct {
	EncryptedUsername string `json:"encrypted_username"`
	UsernameHash      string `json:"username_hash"`
	PasswordSalt      string `json:"password_salt"`
	PasswordHash      string `json:"password_hash"`
	Iterations        int    `json:"iterations"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

// Session represents an authenticated active admin session.
type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AuthManager manages administrative authentication, encryption, and sessions.
type AuthManager struct {
	mu           sync.RWMutex
	rootDir      string
	configFile   string
	secretFile   string
	config       *AuthConfig
	secret       []byte
	sessions     map[string]Session
	cookieDomain string
}

// NewAuthManager initializes or loads the auth manager for the given server root directory.
func NewAuthManager(rootDir string) (*AuthManager, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	mgr := &AuthManager{
		rootDir:    absRoot,
		configFile: filepath.Join(absRoot, authConfigFileName),
		secretFile: filepath.Join(absRoot, secretFileName),
		sessions:   make(map[string]Session),
	}

	if err := mgr.initSecret(); err != nil {
		return nil, fmt.Errorf("failed to init server secret: %w", err)
	}

	_ = mgr.loadConfig()
	return mgr, nil
}

// initSecret loads or generates the persistent 32-byte secret seed used for AES-GCM username encryption.
func (a *AuthManager) initSecret() error {
	if data, err := os.ReadFile(a.secretFile); err == nil && len(data) >= 32 {
		a.secret = data[:32]
		return nil
	}

	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return err
	}
	a.secret = secret

	_ = os.WriteFile(a.secretFile, secret, 0600)
	return nil
}

func (a *AuthManager) loadConfig() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.configFile)
	if err != nil {
		return err
	}

	var cfg AuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	a.config = &cfg
	return nil
}

// IsConfigured returns true if admin credentials have been configured.
func (a *AuthManager) IsConfigured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config != nil && a.config.PasswordHash != "" && a.config.PasswordSalt != ""
}

// SetCredentials encrypts the username and hashes the password, saving to server-auth.json.
func (a *AuthManager) SetCredentials(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if len(password) < 4 {
		return fmt.Errorf("password must be at least 4 characters")
	}

	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("failed to generate cryptographic salt: %w", err)
	}
	saltHex := hex.EncodeToString(salt)

	passHashHex := hashPassword(password, salt, defaultIterations)
	encUser, userHash, err := a.encryptUsername(username, salt)
	if err != nil {
		return fmt.Errorf("failed to encrypt username: %w", err)
	}

	cfg := AuthConfig{
		EncryptedUsername: encUser,
		UsernameHash:      userHash,
		PasswordSalt:      saltHex,
		PasswordHash:      passHashHex,
		Iterations:        defaultIterations,
		CreatedAt:         time.Now().Unix(),
		UpdatedAt:         time.Now().Unix(),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(a.configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to save %s: %w", a.configFile, err)
	}

	a.mu.Lock()
	a.config = &cfg
	// Invalidate older sessions on password change
	a.sessions = make(map[string]Session)
	a.mu.Unlock()

	return nil
}

func (a *AuthManager) deriveKey(salt []byte) []byte {
	h := sha256.New()
	h.Write(a.secret)
	h.Write(salt)
	return h.Sum(nil)
}

func (a *AuthManager) encryptUsername(username string, salt []byte) (string, string, error) {
	key := a.deriveKey(salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(username), nil)
	encStr := base64.StdEncoding.EncodeToString(ciphertext)

	uHash := sha256.Sum256([]byte(strings.ToLower(username)))
	uHashHex := hex.EncodeToString(uHash[:])

	return encStr, uHashHex, nil
}

func (a *AuthManager) decryptUsername() (string, error) {
	if a.config == nil || a.config.EncryptedUsername == "" || a.config.PasswordSalt == "" {
		return "", fmt.Errorf("no credentials configured")
	}

	salt, err := hex.DecodeString(a.config.PasswordSalt)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(a.config.EncryptedUsername)
	if err != nil {
		return "", err
	}

	key := a.deriveKey(salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("invalid ciphertext")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// GetUsername returns the decrypted admin username for display.
func (a *AuthManager) GetUsername() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	u, err := a.decryptUsername()
	if err != nil {
		return "admin"
	}
	return u
}

// Authenticate checks if the supplied username and password are correct.
func (a *AuthManager) Authenticate(username, password string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.config == nil {
		return false
	}

	// 1. Verify username hash in constant time
	inputUHash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	inputUHashHex := hex.EncodeToString(inputUHash[:])
	if subtle.ConstantTimeCompare([]byte(inputUHashHex), []byte(a.config.UsernameHash)) != 1 {
		return false
	}

	// 2. Verify password hash in constant time
	salt, err := hex.DecodeString(a.config.PasswordSalt)
	if err != nil {
		return false
	}

	iter := a.config.Iterations
	if iter <= 0 {
		iter = defaultIterations
	}

	inputPassHashHex := hashPassword(password, salt, iter)
	return subtle.ConstantTimeCompare([]byte(inputPassHashHex), []byte(a.config.PasswordHash)) == 1
}

func hashPassword(password string, salt []byte, iterations int) string {
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	h.Write(salt)
	cur := h.Sum(nil)

	for i := 1; i < iterations; i++ {
		h.Reset()
		h.Write(cur)
		h.Write(salt)
		h.Write([]byte(password))
		cur = h.Sum(nil)
	}

	return hex.EncodeToString(cur)
}

// CreateSession generates a secure session token for an authenticated user.
func (a *AuthManager) CreateSession(username string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)

	now := time.Now()
	sess := Session{
		Token:     token,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(sessionTTL),
	}

	a.mu.Lock()
	a.cleanExpiredSessionsLocked()
	a.sessions[token] = sess
	a.mu.Unlock()

	return token, nil
}

// ValidateToken verifies that a session token is active and not expired.
func (a *AuthManager) ValidateToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}

	a.mu.RLock()
	sess, exists := a.sessions[token]
	a.mu.RUnlock()

	if !exists {
		return "", false
	}

	if time.Now().After(sess.ExpiresAt) {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return "", false
	}

	return sess.Username, true
}

// RevokeToken deletes a session token (logout).
func (a *AuthManager) RevokeToken(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *AuthManager) cleanExpiredSessionsLocked() {
	now := time.Now()
	for k, s := range a.sessions {
		if now.After(s.ExpiresAt) {
			delete(a.sessions, k)
		}
	}
}

// ExtractToken retrieves the session token from an HTTP request (Bearer header, X-Admin-Token, or cookie).
func ExtractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if tok := r.Header.Get("X-Admin-Token"); tok != "" {
		return tok
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// SetSessionCookie sets the HTTP session cookie on response writer.
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ClearSessionCookie clears the HTTP session cookie on response writer.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
