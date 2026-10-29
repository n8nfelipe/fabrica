# Tasks

## 1. Gateway policy and MCP protocol

- [x] 1.1 Implement strict MCP stdio initialization, tool listing, tool calls, request limits, protocol errors, total-call ceiling, and repeated-call loop detection; test protocol errors and both loop limits.
- [x] 1.2 Implement canonical workspace path validation, symlink rejection, sensitive-path filtering, bounded listing/search/read, and atomic writes/deletes; add tests for traversal, symlinks, limits, and denied writes.
- [x] 1.3 Implement phase policies and strict `scope.json` parsing for task-specific specification writes and approved implementation paths; test exact paths, directory prefixes, reserved paths, malformed manifests, and injection strings.

## 2. OpenCode mediation

- [x] 2.1 Add a gateway mode to the application executable and configure each OpenCode 2.1.4 run with only local gateway tools; test generated config/env and validate the V2 config with the installed CLI.
- [x] 2.2 Update specification and implementation prompts to label repository/task/QA content as untrusted data and instruct the agent to use gateway tools only; test the prompt content and task phase parameters.
- [x] 2.3 Capture gateway violations during execution and return a failed result when any denied operation is attempted; test violation audit and cleanup with a fake OpenCode process.
- [x] 2.4 Set the OpenCode child process `PWD` to the canonical project workspace so relative paths cannot resolve against the factory server directory; test with a conflicting inherited `PWD`.

## 3. Approval-bound implementation scope

- [x] 3.1 Require `scope.json` among usable specification artifacts and validate it before PO approval; test absent, invalid, and valid scope manifests.
- [x] 3.2 Add a database migration to persist approved writable paths, store them atomically with specification approval, and load them for implementation; test migration behavior and SQL paths with mocks.
- [x] 3.3 Reject gateway-denied or post-run out-of-scope file changes and keep the task out of QA with redacted failure audit; test allowed, denied, and direct filesystem bypass changes.
- [x] 3.4 Reject specification artifacts from implementation write scopes, give actionable approval feedback for invalid paths, and instruct the specification agent to derive scope from application files.

## 4. Verification

- [x] 4.1 Run backend tests and coverage, `go vet ./...`, frontend lint, and frontend build/typecheck; note that no frontend test script is configured, fix failures, and verify coverage does not decrease.
- [x] 4.2 Validate the OpenSpec change and inspect the final diff for secret exposure, unrestricted tool fallbacks, and untested guard paths.
