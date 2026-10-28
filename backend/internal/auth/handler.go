package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "fabrica_session"
	sessionLifetime   = 12 * time.Hour
	maxRequestSize    = 16 * 1024
	bootstrapLockID   = int64(741928351)
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type Handler struct {
	db *sql.DB
}

type Action string

const (
	ActionManageUsers      Action = "manage_users"
	ActionOrganizeProjects Action = "organize_projects"
	ActionViewProgress     Action = "view_progress"
	ActionApproveSpec      Action = "approve_specification"
	ActionPerformQA        Action = "perform_qa"
	ActionImplement        Action = "implement"
	ActionMerge            Action = "merge"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type setupRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type userResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type Principal struct {
	ID          int64
	Username    string
	DisplayName string
	Role        string
}

func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	h := &Handler{db: db}
	mux.HandleFunc("GET /api/setup/status", h.setupStatus)
	mux.HandleFunc("POST /api/setup/admin", h.createFirstAdmin)
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/session", h.session)
	h.registerUserRoutes(mux)
}

func (h *Handler) registerUserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/users", h.requireAction(ActionManageUsers, h.listUsers))
	mux.HandleFunc("POST /api/users", h.requireAction(ActionManageUsers, h.createUser))
	mux.HandleFunc("GET /api/users/{id}", h.requireAction(ActionManageUsers, h.getUser))
	mux.HandleFunc("PATCH /api/users/{id}", h.requireAction(ActionManageUsers, h.updateUser))
	mux.HandleFunc("POST /api/users/{id}/deactivate", h.requireAction(ActionManageUsers, h.deactivateUser))
}

func (h *Handler) setupStatus(w http.ResponseWriter, r *http.Request) {
	var hasUsers bool
	if err := h.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&hasUsers); err != nil {
		http.Error(w, "could not read setup state", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsAdminSetup": !hasUsers})
}

func (h *Handler) createFirstAdmin(w http.ResponseWriter, r *http.Request) {
	if !requestIsLoopback(r) {
		http.Error(w, "initial setup is available only from loopback", http.StatusForbidden)
		return
	}
	var input setupRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if err := validateCredentials(input.Username, input.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.DisplayName == "" || len(input.DisplayName) > 120 {
		http.Error(w, "displayName must contain between 1 and 120 characters", http.StatusBadRequest)
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "could not secure password", http.StatusInternalServerError)
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "could not initialize admin setup", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock($1)`, bootstrapLockID); err != nil {
		http.Error(w, "could not initialize admin setup", http.StatusInternalServerError)
		return
	}
	var hasUsers bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&hasUsers); err != nil {
		http.Error(w, "could not read setup state", http.StatusInternalServerError)
		return
	}
	if hasUsers {
		http.Error(w, "initial admin is already configured", http.StatusConflict)
		return
	}
	var user userResponse
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO users (username, display_name, password_hash, role)
		VALUES ($1, $2, $3, 'Admin')
		RETURNING id, username, display_name, role
	`, input.Username, input.DisplayName, string(passwordHash)).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role); err != nil {
		http.Error(w, "could not create initial admin", http.StatusConflict)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "could not save initial admin", http.StatusInternalServerError)
		return
	}
	if err := h.createSession(w, r, user); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	var user userResponse
	var passwordHash string
	var active bool
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, username, display_name, role, password_hash, active
		FROM users WHERE username = $1
	`, input.Username).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &passwordHash, &active)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		http.Error(w, "invalid username or password", http.StatusUnauthorized)
		return
	}
	if err := h.createSession(w, r, user); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		hash := hashToken(cookie.Value)
		if _, err := h.db.ExecContext(r.Context(), `DELETE FROM user_sessions WHERE token_hash = $1`, hash[:]); err != nil {
			http.Error(w, "could not end session", http.StatusInternalServerError)
			return
		}
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	user, err := h.userFromRequest(r)
	if err != nil {
		h.clearSessionCookie(w)
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) requireAction(action Action, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := h.userFromRequest(r)
		if err != nil {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !Can(user.Role, action) {
			http.Error(w, "your role cannot perform this action", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func RequireAction(db *sql.DB, action Action, next http.HandlerFunc) http.HandlerFunc {
	return (&Handler{db: db}).requireAction(action, next)
}

func CurrentUser(db *sql.DB, r *http.Request) (Principal, error) {
	user, err := (&Handler{db: db}).userFromRequest(r)
	if err != nil {
		return Principal{}, err
	}
	return Principal{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role}, nil
}

// Can is the authoritative initial role-to-action policy for workflow handlers.
func Can(role string, action Action) bool {
	if role == "Admin" {
		return true
	}
	switch action {
	case ActionOrganizeProjects:
		return role == "PO" || role == "Scrum Master"
	case ActionViewProgress:
		return role == "PO" || role == "QA" || role == "Scrum Master" || role == "Desenvolvedor"
	case ActionApproveSpec:
		return role == "PO"
	case ActionPerformQA:
		return role == "QA"
	case ActionImplement:
		return role == "Desenvolvedor"
	case ActionMerge:
		return role == "PO" || role == "QA" || role == "Desenvolvedor"
	default:
		return false
	}
}

func (h *Handler) userFromRequest(r *http.Request) (userResponse, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return userResponse{}, err
	}
	hash := hashToken(cookie.Value)
	var user userResponse
	err = h.db.QueryRowContext(r.Context(), `
		SELECT u.id, u.username, u.display_name, u.role
		FROM user_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now() AND u.active = TRUE
	`, hash[:]).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role)
	return user, err
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request, user userResponse) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := hashToken(token)
	expiresAt := time.Now().Add(sessionLifetime)
	if _, err := h.db.ExecContext(r.Context(), `INSERT INTO user_sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, hash[:], user.ID, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionLifetime.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateCredentials(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return errors.New("username must be 3 to 64 lowercase letters, numbers, dots, underscores, or hyphens")
	}
	if len(password) < 12 || len(password) > 72 {
		return errors.New("password must contain between 12 and 72 bytes")
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

func hashToken(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
