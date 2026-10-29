package database

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestOpenRejectsMissingAndMalformedConnectionStrings(t *testing.T) {
	if _, err := Open(context.Background(), "  "); err == nil {
		t.Fatal("empty connection string should be rejected")
	}
	if _, err := Open(context.Background(), "postgres:// invalid url"); err == nil {
		t.Fatal("malformed connection string should be rejected")
	}
}

func TestOpenClosesDatabaseWhenPingFails(t *testing.T) {
	db, err := Open(context.Background(), "postgres://test:test@127.0.0.1:1/fabrica?sslmode=disable")
	if err == nil {
		db.Close()
		t.Fatal("Open should fail when PostgreSQL is unreachable")
	}
	if !strings.Contains(err.Error(), "cannot connect to PostgreSQL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrateReportsSchemaHistoryFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).WillReturnError(errors.New("database unavailable"))
	if err := migrate(context.Background(), db); err == nil {
		t.Fatal("migration should report failed schema history creation")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateAppliesEmbeddedMigrationsAndSkipsAppliedOnRerun(t *testing.T) {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, alreadyApplied := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).WillReturnResult(sqlmock.NewResult(0, 0))
		for _, name := range files {
			base := filepath.Base(name)
			versionText, _, _ := strings.Cut(base, "_")
			version, parseErr := strconv.ParseInt(versionText, 10, 64)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM schema_migrations WHERE version = \$1\)`).WithArgs(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(alreadyApplied))
			if alreadyApplied {
				continue
			}
			mock.ExpectBegin()
			mock.ExpectExec(`(?s).*`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`INSERT INTO schema_migrations`).WithArgs(version, base).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		if err := migrate(context.Background(), db); err != nil {
			t.Fatalf("migrate applied=%t: %v", alreadyApplied, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestMigrateReportsVersionAndTransactionFailures(t *testing.T) {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	base := filepath.Base(files[0])
	versionText, _, _ := strings.Cut(base, "_")
	version, err := strconv.ParseInt(versionText, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		configure func(sqlmock.Sqlmock)
	}{
		{name: "version lookup", configure: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(version).WillReturnError(errors.New("query failed"))
		}},
		{name: "begin transaction", configure: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
		}},
		{name: "apply migration", configure: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectBegin()
			mock.ExpectExec(`(?s).*`).WillReturnError(errors.New("migration failed"))
			mock.ExpectRollback()
		}},
		{name: "record migration", configure: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectBegin()
			mock.ExpectExec(`(?s).*`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`INSERT INTO schema_migrations`).WithArgs(version, base).WillReturnError(errors.New("record failed"))
			mock.ExpectRollback()
		}},
		{name: "commit migration", configure: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`SELECT EXISTS`).WithArgs(version).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectBegin()
			mock.ExpectExec(`(?s).*`).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`INSERT INTO schema_migrations`).WithArgs(version, base).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).WillReturnResult(sqlmock.NewResult(0, 0))
			tc.configure(mock)
			if err := migrate(context.Background(), db); err == nil {
				t.Fatal("expected migration failure")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
