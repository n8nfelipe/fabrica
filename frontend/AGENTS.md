# Repository Guidelines

## Project Structure & Module Organization

The repository has two application modules: `backend/` contains the Go API, domain packages, PostgreSQL migrations, and package-level tests; `frontend/` contains the React and TypeScript UI, Vite configuration, and static assets in `public/`. Product and development documentation lives in `docs/`; OpenSpec specifications and changes live in `openspec/`; local operational scripts live in `scripts/`. Keep tests beside the package or feature they exercise, using `*_test.go` for Go tests.

## Build, Test, and Development Commands

- `docker compose up -d --wait` starts the local PostgreSQL service.
- `./scripts/run-api.sh` loads `.env`, applies migrations, and starts the API on `127.0.0.1:8080`.
- `cd frontend && npm ci && npm run dev` installs frontend dependencies and starts Vite on port `5173`.
- `cd backend && go test ./...` runs all Go tests.
- `cd frontend && npm run lint && npm run build` checks ESLint rules, TypeScript, and the production Vite build.

## Coding Style & Naming Conventions

Format Go code with `gofmt`; use Go's standard exported `PascalCase` and unexported `camelCase` names. Frontend code uses TypeScript/TSX, two-space indentation, `PascalCase` for React components and types, and `camelCase` for functions and values. Keep styles in the existing CSS files and follow the project's ESLint configuration. Avoid adding dependencies when the existing Go or npm packages already cover the need.

## Testing Guidelines

Go tests use the standard `testing` package and are named `Test...`; run the affected package with `go test ./internal/<package>` before the full suite. Integration tests may require Docker or a configured test database and can skip when their documented environment variables are absent. The frontend currently provides lint and build checks rather than a separate test script.

## Commit & Pull Request Guidelines

Recent commits use concise imperative subjects; examples include `fix: satisfy frontend lint rule` and `docs: add project README`. A `fix:` or `docs:` prefix is useful, though the history is not strictly Conventional Commits. Pull requests should explain the change and its verification, link related work when applicable, and include screenshots for visible UI changes. Call out configuration or migration effects.

## Security & Local Configuration

Copy `.env.example` to `.env`, set a strong `POSTGRES_PASSWORD`, and keep `.env` and credentials out of commits. The API, database, and QA environments are local services; bind exposed development ports to loopback unless a requirement explicitly calls for broader access.
