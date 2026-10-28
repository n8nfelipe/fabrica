# Design

## Context

See [proposal.md](proposal.md) for motivation and the five new capabilities. The repository currently contains the OpenSpec configuration and PRD but no application source, API contracts, or existing capabilities. The MVP runs on one local machine. The user's OpenCode installation, Docker Engine, Docker Compose, Git, and authenticated `gh` session are available on that machine. The factory's own PostgreSQL and each project's database run in Docker containers.

## Goals / Non-Goals

**Goals:**

- Keep the factory's control plane local and able to access the host workspace, OpenCode, GitHub CLI, and Docker.
- Keep project state and human approvals durable across process restarts.
- Make agent work resumable and auditable without allowing an agent to pass PO or QA gates.
- Support a free-form project stack while keeping execution within Docker Compose and providing a local QA URL.

**Non-Goals:**

- Remote team access, cloud execution, production deployment, or a hosted agent service in this MVP.
- Guaranteeing that arbitrary technologies run without Docker Compose support or project-specific configuration.
- Automatic provisioning of hosted databases or GitHub organizations/repositories.

## Decisions

### Local control plane with host-side runner

Run the Go API and task runner as local processes and serve the React UI locally. Run the factory PostgreSQL in a dedicated Docker Compose stack. Keep the runner on the host so it can use the existing OpenCode and `gh` installations and their user authentication, operate `~/workspace`, and invoke Docker Compose. Do not put `gh` tokens or the Docker socket inside the browser-facing process/container.

**Alternative considered:** put the entire factory in containers and mount the Docker socket, workspace, OpenCode configuration, and GitHub CLI credentials into the API container. This complicates host credential reuse and grants the web-facing service broad host control.

### PostgreSQL as the factory's source of truth

Persist users, projects, tasks, task transitions, approvals, agent runs, environment metadata, and pull request references in PostgreSQL. Use database transactions for task-state transitions and append-only event records for audit history. Version schema migrations and use a dedicated Compose project/volume for factory data.

Each generated project owns an independent Compose project, service namespace, and data volume. Its database engine and configuration come from that project's free-form stack requirements; when the stack includes a database, it runs as a Compose service on the local Docker Engine.

### Local accounts and initial administrator

Use individual factory accounts with local credentials for the MVP. Store only strong password hashes; never expose credentials in logs. When the user table is empty, show a one-time setup screen bound to loopback to create the first Admin, then disable that route. Do not provide a shared default password.

**Alternative considered:** rely on an external identity provider. It adds a service dependency to a single-machine MVP and does not fit the local-only scope.

Initial Scrum Master permissions are limited to organizing projects and tasks and viewing progress. The role cannot manage users, approve specifications or QA, or merge until the PO defines a broader matrix. Admin remains unrestricted; the explicit PO, QA, and Developer actions follow `user-access`.

### State machine and human gates

Treat the task state in PostgreSQL as canonical. Trigger an agent run only from an allowed state transition. A successful specification run moves a task to PO review; only the PO approval event enables implementation. A successful implementation starts the Compose environment and opens QA; only a QA approval event enables PR creation. PR checks gate merge; successful merge marks the task complete. Failures leave the task in its current phase with a recorded run/error and an authorized retry path.

Use idempotency keys derived from task, phase, and attempt to prevent duplicate agent runs or duplicate PR creation after retries. Persist an operation record before invoking external side effects, then reconcile its result after process restart.

### OpenCode integration

Invoke the installed OpenCode CLI from the host runner in the target project's workspace. Use its non-interactive `run` mode, pass a task-specific prompt through stdin, request JSON event output, and record the process exit status and final output. The current CLI documentation describes `opencode run` for non-interactive prompts, target-directory execution, and `--format json` for structured events ([CLI docs](https://opencode.ai/docs/cli/), [command docs](https://opencode.ai/docs/cli/commands/)).

Use separate prompts for specification and implementation. The specification prompt asks for SDD/OpenSpec artifacts and acceptance scenarios; it must not authorize implementation. The implementation prompt includes only the PO-approved artifacts and task context. Scope each process to the selected workspace, capture output with size limits, redact known secrets, and terminate a run when the user cancels it. Do not enable permission-bypass flags by default.

**Alternative considered:** integrate through a remote OpenCode server/API. The local CLI matches the user's installed agent and avoids adding a network service to the MVP.

### Project descriptor and Compose lifecycle

Keep the free-form stack description as project input, then have the agent produce the project's Compose file plus a small factory descriptor that identifies the Compose file, QA service, container port, and optional health check. This descriptor is the stable interface between generated projects and the factory; it avoids hard-coding React or any other project's framework into the runner.

Start each project under a unique Compose project name derived from the factory project ID. Validate the descriptor and Compose configuration before startup. Route QA to the declared service. Assign a distinct host port from a managed local pool, check both host sockets and factory-managed allocations, and retry startup with another port if Docker reports a bind collision. Persist the active URL and stop only the Compose project associated with the task. The factory itself serves its UI/API locally and uses a separate Compose project for PostgreSQL.

**Alternative considered:** require every generated project to use a predefined template and service port. That would simplify environment discovery but conflict with the user's decision to keep project stacks free-form.

### GitHub delivery through `gh`

Use the host's authenticated GitHub CLI for repository operations. Check authentication at setup and before a GitHub operation; do not copy tokens into factory settings. Create the PR only after QA approval, include task/specification references, and persist its URL and number. Poll/check status with machine-readable output; only required successful checks make the task merge-eligible. The CLI supports non-interactive PR creation, JSON check output, and merge commands ([`gh pr create`](https://cli.github.com/manual/gh_pr_create), [`gh pr checks`](https://cli.github.com/manual/gh_pr_checks), [`gh pr merge`](https://cli.github.com/manual/gh_pr_merge), [`gh auth status`](https://cli.github.com/manual/gh_auth_status)).

Do not let the agent merge automatically. Present a merge action only to Admin, Developer, QA, or PO and invoke `gh pr merge` after checking required status again. Admin's ability to manage factory actions does not mean bypassing GitHub's required checks.

The repository workflows preserve the project CI contract: `ci.yml` triggers only on `pull_request` and runs tests, lint, typecheck, and static analysis; `release.yml` triggers only on push to `main` and creates an annotated SemVer tag, starting at `v1.0.0`. Release does not rerun checks, and CI does not create tags.

### Failure handling and local security boundary

Keep the web API bound to loopback for the MVP. Restrict filesystem operations to the configured workspace root and reject path traversal or a project name that resolves outside it. Keep project Compose networks and volumes isolated by project ID. The local runner can execute arbitrary project code through OpenCode and Docker, so it is a trusted local operator, not a multi-tenant sandbox; require the operator to approve each task phase at the specified gates.

On factory restart, reconcile persisted task operations with running Compose projects and GitHub PR status. Do not infer success solely from a stale process record. Preserve redacted logs needed for recovery; environment retention and automatic cleanup duration remain configurable later.

## Risks / Trade-offs

- **OpenCode and project code can modify or execute files in the workspace** → Restrict the runner's project paths, use a separate Git worktree/branch per task, record changed files, and require PO/QA gates before delivery.
- **A local runner can control Docker and invoke host commands** → Bind the control plane to loopback, use argument arrays rather than shell interpolation, validate workspace paths, and document that this MVP is for a trusted local machine.
- **Free-form stacks may not expose a clear QA service or health endpoint** → Require a generated project descriptor and block QA until Compose health and the declared service port are validated.
- **Host ports can become occupied between detection and bind** → Reserve allocations in the factory, retry Compose startup on bind conflicts, and persist the final assigned port only after successful startup.
- **The user's `gh` authentication may expire** → Check auth state at operation time and leave the task retryable without losing approvals or branch work.
- **Local DB volume loss would erase workflow history** → Use a named persistent Docker volume and document a backup/export procedure before operational rollout.

## Migration Plan

This is a greenfield application; there is no production data to migrate. Initial setup creates the local PostgreSQL volume and schema, then the one-time Admin. Rollback for the MVP is to stop the factory and project Compose stacks while preserving named volumes. Schema changes use reversible migrations where practical; any irreversible data change requires a backup before applying.

## Open Questions

- Confirm the final Scrum Master permission matrix before expanding beyond project/task organization and read-only progress.
- Decide project environment retention defaults after observing local disk use; initially allow manual stop/cleanup and keep completed task history.
- Define expected project concurrency and the default local port pool before implementation capacity is tuned.

## References

- [OpenCode CLI documentation](https://opencode.ai/docs/cli/) and [CLI commands](https://opencode.ai/docs/cli/commands/), consulted 2026-10-27 via Context7.
- [GitHub CLI: create PR](https://cli.github.com/manual/gh_pr_create), [check PR status](https://cli.github.com/manual/gh_pr_checks), [merge PR](https://cli.github.com/manual/gh_pr_merge), and [auth status](https://cli.github.com/manual/gh_auth_status), consulted 2026-10-27 via Context7.
- Project CI/CD contract: `.agents/skills/regra-ci/SKILL.md`.
