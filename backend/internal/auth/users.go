package auth

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var availableRoles = map[string]struct{}{
	"PO": {}, "QA": {}, "Admin": {}, "Scrum Master": {}, "Desenvolvedor": {},
}

type managedUser struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"createdAt"`
}

type createUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

type updateUserRequest struct {
	Username    *string `json:"username"`
	DisplayName *string `json:"displayName"`
	Password    *string `json:"password"`
	Role        *string `json:"role"`
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `SELECT id, username, display_name, role, active, created_at FROM users ORDER BY id`)
	if err != nil {
		http.Error(w, "could not list users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	users := make([]managedUser, 0)
	for rows.Next() {
		var user managedUser
		var createdAt sql.NullTime
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.Active, &createdAt); err != nil {
			http.Error(w, "could not read users", http.StatusInternalServerError)
			return
		}
		if createdAt.Valid {
			user.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read users", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUserID(w, r)
	if !ok {
		return
	}
	user, err := h.readManagedUser(r, id)
	if err == sql.ErrNoRows {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var input createUserRequest
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
	if !validDisplayName(input.DisplayName) {
		http.Error(w, "displayName must contain between 1 and 120 characters", http.StatusBadRequest)
		return
	}
	if !validRole(input.Role) {
		http.Error(w, "role must be one of PO, QA, Admin, Scrum Master, or Desenvolvedor", http.StatusBadRequest)
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "could not secure password", http.StatusInternalServerError)
		return
	}
	var user userResponse
	err = h.db.QueryRowContext(r.Context(), `
		INSERT INTO users (username, display_name, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, username, display_name, role
	`, input.Username, input.DisplayName, string(passwordHash), input.Role).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role)
	if err != nil {
		http.Error(w, "could not create user; username may already be in use", http.StatusConflict)
		return
	}
	managed, err := h.readManagedUser(r, user.ID)
	if err != nil {
		http.Error(w, "user created but could not be read", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, managed)
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUserID(w, r)
	if !ok {
		return
	}
	var input updateUserRequest
	if err := decodeJSON(w, r, &input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if input.Username == nil && input.DisplayName == nil && input.Password == nil && input.Role == nil {
		http.Error(w, "at least one user field must be provided", http.StatusBadRequest)
		return
	}
	current, err := h.readManagedUser(r, id)
	if err == sql.ErrNoRows {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "could not read user", http.StatusInternalServerError)
		return
	}
	username := current.Username
	if input.Username != nil {
		username = strings.ToLower(strings.TrimSpace(*input.Username))
		if !usernamePattern.MatchString(username) {
			http.Error(w, "username must be 3 to 64 lowercase letters, numbers, dots, underscores, or hyphens", http.StatusBadRequest)
			return
		}
	}
	displayName := current.DisplayName
	if input.DisplayName != nil {
		displayName = strings.TrimSpace(*input.DisplayName)
		if !validDisplayName(displayName) {
			http.Error(w, "displayName must contain between 1 and 120 characters", http.StatusBadRequest)
			return
		}
	}
	role := current.Role
	if input.Role != nil {
		role = strings.TrimSpace(*input.Role)
		if !validRole(role) {
			http.Error(w, "role must be one of PO, QA, Admin, Scrum Master, or Desenvolvedor", http.StatusBadRequest)
			return
		}
	}
	passwordHash := ""
	if input.Password != nil {
		if len(*input.Password) < 12 || len(*input.Password) > 72 {
			http.Error(w, "password must contain between 12 and 72 bytes", http.StatusBadRequest)
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(*input.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "could not secure password", http.StatusInternalServerError)
			return
		}
		passwordHash = string(hashed)
	}
	result, err := h.db.ExecContext(r.Context(), `
		UPDATE users
		SET username = $2, display_name = $3, role = $4,
		    password_hash = CASE WHEN $5 = '' THEN password_hash ELSE $5 END,
		    updated_at = now()
		WHERE id = $1
	`, id, username, displayName, role, passwordHash)
	if err != nil {
		http.Error(w, "could not update user; username may already be in use", http.StatusConflict)
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	updated, err := h.readManagedUser(r, id)
	if err != nil {
		http.Error(w, "user updated but could not be read", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUserID(w, r)
	if !ok {
		return
	}
	result, err := h.db.ExecContext(r.Context(), `UPDATE users SET active = FALSE, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "could not deactivate user", http.StatusInternalServerError)
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) readManagedUser(r *http.Request, id int64) (managedUser, error) {
	var user managedUser
	var createdAt sql.NullTime
	err := h.db.QueryRowContext(r.Context(), `SELECT id, username, display_name, role, active, created_at FROM users WHERE id = $1`, id).
		Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.Active, &createdAt)
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	return user, err
}

func parseUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func validDisplayName(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 120
}

func validRole(role string) bool {
	_, ok := availableRoles[role]
	return ok
}
