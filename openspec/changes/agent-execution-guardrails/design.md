# Design

## Context

OpenCode currently runs from the project workspace with its normal tool set, while the backend only checks the workspace root and records a before/after snapshot. OpenCode permission rules are not an operating-system sandbox. The workflow already has PO approval before implementation and backend-owned QA/GitHub actions.

## Goals / Non-Goals

**Goals:**
- Make a backend-owned MCP gateway the only tool surface available to specification and implementation runs.
- Deny command execution, network tools, subagents, project-defined tools, and direct file tools for agent runs.
- Validate all gateway paths against the canonical workspace, reject symlinks and sensitive paths, and apply phase-specific write rules.
- Have the PO approve a machine-readable implementation write scope and persist that scope in the database before implementation starts.
- Preserve auditability and block workflow advancement after gateway or post-run scope violations.

**Non-Goals:**
- Sandboxing project code that the backend intentionally runs later through Docker Compose for QA.
- Giving the agent a general-purpose shell, package manager, network client, GitHub client, or arbitrary command tool.
- Claiming that a prompt or the OpenCode tool permission layer isolates a compromised host process.

## Decisions

### Expose a local MCP gateway as the only agent tool surface

The Go application binary will support a stdio MCP mode. For the installed OpenCode 2.1.4 CLI, each run will receive a temporary V2 configuration using `OPENCODE_CONFIG_CONTENT`, `permissions`, `mcp.servers`, and a dedicated primary `fabrica` agent. The executor disables project configuration/instruction files, external skills, and plugins, and registers one local gateway server with Code Mode off. The gateway exposes only `list_files`, `search_files`, `read_file`, `write_file`, and `delete_file`. It has no shell, network, Git, QA, or GitHub operation. The OpenCode run still uses its normal provider connection; the gateway itself does not receive provider credentials as tool data.

Use MCP stdio rather than HTTP so no listener or network endpoint is exposed. Keep the protocol implementation small and strict: support initialization, ping, tool listing, and tool calls; reject unknown methods and malformed requests. The V2 policy denies every action globally and repeats the deny/allow list on the dedicated primary agent, allowing only normalized `fabrica_gateway_*` tools. Pin configuration keys to the OpenCode CLI version supported by the project and test the generated config as data and against the installed CLI parser.

Pass the canonical project workspace as the child process's `PWD` as well as its OS working directory. OpenCode CLI resolves the run root from `PWD` before falling back to the process working directory, so inheriting the factory server's `PWD` would make relative paths target the factory repository.

**Alternative considered:** rely on prompt rules or OpenCode confirmation permissions. They do not provide a backend-owned enforcement point and do not isolate the process.

### Persist the approved write scope

Specification generation must produce `openspec/changes/task-<id>/scope.json` with version 1 and a non-empty list of workspace-relative exact application paths or directory prefixes ending in `/`. These paths describe implementation writes only and MUST NOT include any `openspec/changes/task-*/` directory, including the task's own specification artifacts. Reject absolute paths, traversal, backslashes, reserved control paths, sensitive files, duplicates, and excessive entries. The PO reviews this artifact with the rest of the specification. At approval, validate and copy the paths into a JSONB column on the task specification row. Implementation reads the persisted scope, never a mutable file in the workspace.

During specification phase, writes are limited to that task's OpenSpec directory. During implementation, writes and deletes must match the persisted exact path or directory prefix. Reads are limited to regular non-sensitive files within the project workspace. All path resolution checks each existing component with `Lstat`, rejects symlinks, and never follows them. Post-run snapshots are checked against the same write scope so a path modified outside the gateway also fails the run.

**Alternative considered:** trust the current scope file at implementation time. The agent could change the file after PO approval, so the approval-time copy is authoritative.

### Treat a denied call as a failed run

The gateway records a violation marker in a private temporary audit file when a tool call attempts a forbidden path or capability. The executor reads and removes this file after OpenCode exits and returns a security error if any violation occurred. The project handler persists a failed run and does not advance the task. A post-run snapshot check provides a second check for file changes that bypassed the gateway.

Store only violation category and normalized relative path, never file contents. Continue redacting process output through the existing path.

The gateway also enforces a fixed maximum tool-call count per run and a lower maximum for repeated calls with identical tool and arguments. The existing task-run context deadline remains the wall-clock limit. Either gateway limit fails the run rather than asking the agent to retry indefinitely.

## Risks / Trade-offs

- **The agent cannot run tests or shell commands inside its turn** → Keep execution tools unavailable; run project tests through the trusted CI/QA workflow and report that constraint in the prompt.
- **An overly broad approved directory prefix weakens scope** → Show `scope.json` as a normal PO-review artifact, validate it strictly, and make the exact manifest persisted and available in audit records.
- **The CLI configuration format changes between OpenCode versions** → Isolate config generation behind a tested function and pin the compatible CLI version in project setup documentation/configuration.
- **The host application binary or gateway is compromised** → The gateway limits model-requested tool actions but is not an OS sandbox; retain the local trusted-operator boundary and avoid describing it as full process isolation.
- **QA Compose can execute project-controlled code after implementation** → Keep QA behind the existing backend gate and treat Docker/Compose execution as a distinct trusted operation.

## Migration Plan

Add a nullable/default-empty JSONB write-scope column to task specifications. Existing approved specifications without a persisted scope cannot start a new implementation attempt; the PO must generate and approve a new specification. New specification generation requires `scope.json`. Rollback can leave the column unused and restore the prior executor, but must not silently permit unrestricted writes for runs created under the gateway policy.
