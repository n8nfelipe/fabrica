package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestSessionAndSetupHandlersWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	w := httptest.NewRecorder()
	h.setupStatus(w, httptest.NewRequest("GET", "/api/setup/status", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"needsAdminSetup":true`) {
		t.Fatalf("setupStatus=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.session(w, httptest.NewRequest("GET", "/api/session", nil))
	if w.Code != 401 {
		t.Fatalf("anonymous session=%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.createFirstAdmin(w, httptest.NewRequest("POST", "/api/setup/admin", strings.NewReader(`{}`)))
	if w.Code != 403 {
		t.Fatalf("remote bootstrap status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedUserReadAndInvalidIDsWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "active", "created_at"}).AddRow(4, "qa.user", "QA User", "QA", true, time.Now()))
	r := httptest.NewRequest("GET", "/api/users/4", nil)
	r.SetPathValue("id", "4")
	w := httptest.NewRecorder()
	h.getUser(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "qa.user") {
		t.Fatalf("getUser=%d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/api/users/invalid", nil)
	r.SetPathValue("id", "invalid")
	w = httptest.NewRecorder()
	h.getUser(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid id status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetUserMapsMissingAndDatabaseErrorWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	query := `SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`
	mock.ExpectQuery(query).WithArgs(int64(90)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "active", "created_at"}))
	r := httptest.NewRequest("GET", "/api/users/90", nil)
	r.SetPathValue("id", "90")
	w := httptest.NewRecorder()
	h.getUser(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing user status=%d", w.Code)
	}
	mock.ExpectQuery(query).WithArgs(int64(91)).WillReturnError(errors.New("database unavailable"))
	r = httptest.NewRequest("GET", "/api/users/91", nil)
	r.SetPathValue("id", "91")
	w = httptest.NewRecorder()
	h.getUser(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("database error status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserListAndMutationValidationWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "active", "created_at"}).AddRow(9, "dev.user", "Developer", "Desenvolvedor", true, time.Now()))
	w := httptest.NewRecorder()
	h.listUsers(w, httptest.NewRequest("GET", "/api/users", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "dev.user") {
		t.Fatalf("listUsers=%d %s", w.Code, w.Body.String())
	}

	invalid := []struct {
		method, body string
		call         func(http.ResponseWriter, *http.Request)
	}{
		{"POST", `{"username":"bad name","displayName":"Bad","password":"long-enough-password","role":"QA"}`, h.createUser},
		{"POST", `{"username":"valid.user","displayName":"","password":"long-enough-password","role":"QA"}`, h.createUser},
		{"POST", `{"username":"valid.user","displayName":"Valid","password":"long-enough-password","role":"root"}`, h.createUser},
		{"PATCH", `{}`, h.updateUser},
		{"PATCH", `{"username":"Bad Name"}`, h.updateUser},
		{"PATCH", `{"role":"root"}`, h.updateUser},
		{"PATCH", `{"password":"short"}`, h.updateUser},
	}
	for _, tc := range invalid {
		if tc.method == "PATCH" && tc.body != `{}` {
			mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(9)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "active", "created_at"}).AddRow(9, "dev.user", "Developer", "Desenvolvedor", true, time.Now()))
		}
		r := httptest.NewRequest(tc.method, "/api/users/9", strings.NewReader(tc.body))
		r.SetPathValue("id", "9")
		w = httptest.NewRecorder()
		tc.call(w, r)
		if w.Code != 400 {
			t.Errorf("%s %s status=%d body=%s", tc.method, tc.body, w.Code, w.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeactivateUserWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectExec(`UPDATE users SET active = FALSE, updated_at = now\(\) WHERE id = \$1`).WithArgs(int64(12)).WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/api/users/12/deactivate", nil)
	r.SetPathValue("id", "12")
	w := httptest.NewRecorder()
	h.deactivateUser(w, r)
	if w.Code != 204 {
		t.Fatalf("deactivateUser=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAndUpdateUserWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	now := time.Now()
	managedColumns := []string{"id", "username", "display_name", "role", "active", "created_at"}
	mock.ExpectQuery(`INSERT INTO users \(username, display_name, password_hash, role\)`).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(14, "new.user", "New User", "QA"))
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(14)).WillReturnRows(sqlmock.NewRows(managedColumns).AddRow(14, "new.user", "New User", "QA", true, now))
	r := httptest.NewRequest("POST", "/api/users", strings.NewReader(`{"username":" New.User ","displayName":" New User ","password":"long-enough-password","role":"QA"}`))
	w := httptest.NewRecorder()
	h.createUser(w, r)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "new.user") {
		t.Fatalf("createUser=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(14)).WillReturnRows(sqlmock.NewRows(managedColumns).AddRow(14, "new.user", "New User", "QA", true, now))
	mock.ExpectExec(`UPDATE users\s+SET username = \$2`).WithArgs(int64(14), "new.user", "Changed", "Admin", "").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(14)).WillReturnRows(sqlmock.NewRows(managedColumns).AddRow(14, "new.user", "Changed", "Admin", true, now))
	r = httptest.NewRequest("PATCH", "/api/users/14", strings.NewReader(`{"displayName":" Changed ","role":"Admin"}`))
	r.SetPathValue("id", "14")
	w = httptest.NewRecorder()
	h.updateUser(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Changed") {
		t.Fatalf("updateUser=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginAndLogoutWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("long-enough-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT id, username, display_name, role, password_hash, active\s+FROM users WHERE username = \$1`).WithArgs("qa.user").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "password_hash", "active"}).AddRow(3, "qa.user", "QA User", "QA", string(passwordHash), true))
	mock.ExpectExec(`INSERT INTO user_sessions \(token_hash, user_id, expires_at\)`).WithArgs(sqlmock.AnyArg(), int64(3), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":" QA.User ","password":"long-enough-password"}`))
	w := httptest.NewRecorder()
	h.login(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].Name != sessionCookieName {
		t.Fatalf("login=%d cookies=%v body=%s", w.Code, w.Result().Cookies(), w.Body.String())
	}

	mock.ExpectExec(`DELETE FROM user_sessions WHERE token_hash = \$1`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	r = httptest.NewRequest("POST", "/api/logout", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-to-remove"})
	w = httptest.NewRecorder()
	h.logout(w, r)
	if w.Code != 204 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
		t.Fatalf("logout=%d cookies=%v", w.Code, w.Result().Cookies())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireActionRejectsAnonymousAndForbiddenRoles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.requireAction(ActionManageUsers, next)(w, r)
	if w.Code != 401 {
		t.Fatalf("anonymous role status=%d", w.Code)
	}
	token := "qa-session"
	hash := hashToken(token)
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(2, "qa.user", "QA User", "QA"))
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w = httptest.NewRecorder()
	h.requireAction(ActionManageUsers, next)(w, r)
	if w.Code != 403 {
		t.Fatalf("forbidden role status=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFirstAdminSetupSuccessAndDuplicateWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	body := `{"username":" First.Admin ","displayName":"First Admin","password":"long-enough-password"}`
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(bootstrapLockID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO users \(username, display_name, password_hash, role\)`).WithArgs("first.admin", "First Admin", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(1, "first.admin", "First Admin", "Admin"))
	mock.ExpectCommit()
	mock.ExpectExec(`INSERT INTO user_sessions \(token_hash, user_id, expires_at\)`).WithArgs(sqlmock.AnyArg(), int64(1), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	r := httptest.NewRequest("POST", "/api/setup/admin", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	h.createFirstAdmin(w, r)
	if w.Code != 201 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("first setup=%d %s", w.Code, w.Body.String())
	}

	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(bootstrapLockID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	r = httptest.NewRequest("POST", "/api/setup/admin", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	w = httptest.NewRecorder()
	h.createFirstAdmin(w, r)
	if w.Code != 409 {
		t.Fatalf("duplicate setup status=%d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFirstAdminRejectsMalformedAndInvalidBootstrapRequests(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	cases := []struct {
		name, body string
		want       int
	}{
		{name: "malformed JSON", body: "{", want: 400},
		{name: "invalid username", body: `{"username":"bad name","displayName":"Admin","password":"long-enough-password"}`, want: 400},
		{name: "short password", body: `{"username":"admin.user","displayName":"Admin","password":"short"}`, want: 400},
		{name: "empty display name", body: `{"username":"admin.user","displayName":" ","password":"long-enough-password"}`, want: 400},
		{name: "long display name", body: `{"username":"admin.user","displayName":"` + strings.Repeat("A", 121) + `","password":"long-enough-password"}`, want: 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/setup/admin", strings.NewReader(tc.body))
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			h.createFirstAdmin(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestFirstAdminReportsBootstrapDatabaseFailuresWithMocks(t *testing.T) {
	body := `{"username":"admin.user","displayName":"Admin User","password":"long-enough-password"}`
	cases := []struct {
		name    string
		prepare func(sqlmock.Sqlmock)
	}{
		{name: "begin", prepare: func(m sqlmock.Sqlmock) { m.ExpectBegin().WillReturnError(errors.New("begin failed")) }},
		{name: "advisory lock", prepare: func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnError(errors.New("lock failed"))
			m.ExpectRollback()
		}},
		{name: "users lookup", prepare: func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnError(errors.New("lookup failed"))
			m.ExpectRollback()
		}},
		{name: "admin insert", prepare: func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			m.ExpectQuery(`INSERT INTO users \(username, display_name, password_hash, role\)`).WillReturnError(errors.New("insert failed"))
			m.ExpectRollback()
		}},
		{name: "commit", prepare: func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			m.ExpectQuery(`INSERT INTO users \(username, display_name, password_hash, role\)`).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(1, "admin.user", "Admin User", "Admin"))
			m.ExpectCommit().WillReturnError(errors.New("commit failed"))
		}},
		{name: "session insert", prepare: func(m sqlmock.Sqlmock) {
			m.ExpectBegin()
			m.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			m.ExpectQuery(`INSERT INTO users \(username, display_name, password_hash, role\)`).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(1, "admin.user", "Admin User", "Admin"))
			m.ExpectCommit()
			m.ExpectExec(`INSERT INTO user_sessions`).WillReturnError(errors.New("session failed"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			tc.prepare(mock)
			h := &Handler{db: db}
			r := httptest.NewRequest("POST", "/api/setup/admin", strings.NewReader(body))
			r.RemoteAddr = "127.0.0.1:1234"
			w := httptest.NewRecorder()
			h.createFirstAdmin(w, r)
			if w.Code < 400 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRegisterRoutesWiresAuthenticationEndpoints(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_ = mock
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	for _, route := range []string{"GET /api/setup/status", "POST /api/setup/admin", "POST /api/login", "POST /api/logout", "GET /api/session", "GET /api/users", "POST /api/users"} {
		if _, pattern := mux.Handler(httptest.NewRequest(strings.Fields(route)[0], strings.Fields(route)[1], nil)); pattern != route {
			t.Errorf("route %q resolved to %q", route, pattern)
		}
	}
}

func TestCurrentUserAndExportedRequireActionWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	token := "current-user-token"
	hash := hashToken(token)
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(6, "dev.user", "Developer", "Desenvolvedor"))
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	user, err := CurrentUser(db, r)
	if err != nil || user.ID != 6 || user.Role != "Desenvolvedor" {
		t.Fatalf("CurrentUser=%#v %v", user, err)
	}
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(hash[:]).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role"}).AddRow(6, "dev.user", "Developer", "Desenvolvedor"))
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	RequireAction(db, ActionImplement, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })(w, r)
	if w.Code != 204 {
		t.Fatalf("RequireAction status=%d body=%s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationHandlersReturnSafeDatabaseErrorsWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM users\)`).WillReturnError(errors.New("database unavailable"))
	w := httptest.NewRecorder()
	h.setupStatus(w, httptest.NewRequest("GET", "/api/setup/status", nil))
	if w.Code != 500 {
		t.Fatalf("setup failure=%d", w.Code)
	}
	r := httptest.NewRequest("POST", "/api/login", strings.NewReader("{"))
	w = httptest.NewRecorder()
	h.login(w, r)
	if w.Code != 400 {
		t.Fatalf("malformed login=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT id, username, display_name, role, password_hash, active\s+FROM users WHERE username = \$1`).WithArgs("unknown.user").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "password_hash", "active"}))
	r = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"unknown.user","password":"long-enough-password"}`))
	w = httptest.NewRecorder()
	h.login(w, r)
	if w.Code != 401 {
		t.Fatalf("unknown login=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT id, username, display_name, role, password_hash, active\s+FROM users WHERE username = \$1`).WithArgs("inactive.user").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "password_hash", "active"}).AddRow(4, "inactive.user", "Inactive", "QA", "invalid hash", false))
	r = httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"inactive.user","password":"long-enough-password"}`))
	w = httptest.NewRecorder()
	h.login(w, r)
	if w.Code != 401 {
		t.Fatalf("inactive login=%d", w.Code)
	}
	mock.ExpectExec(`DELETE FROM user_sessions WHERE token_hash = \$1`).WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("write failed"))
	r = httptest.NewRequest("POST", "/api/logout", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "logout-error"})
	w = httptest.NewRecorder()
	h.logout(w, r)
	if w.Code != 500 {
		t.Fatalf("logout error=%d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/logout", nil)
	w = httptest.NewRecorder()
	h.logout(w, r)
	if w.Code != 204 {
		t.Fatalf("anonymous logout=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT u.id, u.username, u.display_name, u.role\s+FROM user_sessions`).WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("database unavailable"))
	r = httptest.NewRequest("GET", "/api/session", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bad-session"})
	w = httptest.NewRecorder()
	h.session(w, r)
	if w.Code != 401 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("bad session=%d cookies=%d", w.Code, len(w.Result().Cookies()))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserHandlersReportQueryErrorsAndMissingTargetsWithMockDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := &Handler{db: db}
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users ORDER BY id`).WillReturnError(errors.New("database unavailable"))
	w := httptest.NewRecorder()
	h.listUsers(w, httptest.NewRequest("GET", "/api/users", nil))
	if w.Code != 500 {
		t.Fatalf("list users database error=%d", w.Code)
	}
	mock.ExpectQuery(`SELECT id, username, display_name, role, active, created_at FROM users WHERE id = \$1`).WithArgs(int64(99)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "display_name", "role", "active", "created_at"}))
	r := httptest.NewRequest("PATCH", "/api/users/99", strings.NewReader(`{"displayName":"Changed"}`))
	r.SetPathValue("id", "99")
	w = httptest.NewRecorder()
	h.updateUser(w, r)
	if w.Code != 404 {
		t.Fatalf("update missing user=%d", w.Code)
	}
	mock.ExpectExec(`UPDATE users SET active = FALSE, updated_at = now\(\) WHERE id = \$1`).WithArgs(int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))
	r = httptest.NewRequest("POST", "/api/users/99/deactivate", nil)
	r.SetPathValue("id", "99")
	w = httptest.NewRecorder()
	h.deactivateUser(w, r)
	if w.Code != 404 {
		t.Fatalf("deactivate missing user=%d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
