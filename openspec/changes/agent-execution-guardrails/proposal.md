# Proposal

## Why

Agent prompts combine task data with repository content and QA feedback, so prompt injection can redirect work or request unrelated actions. Tool permissions and prompt rules alone do not provide a trusted enforcement point; the runner needs a gateway that mediates the agent's available tools and verifies its write scope.

## What Changes

- Add a local MCP security gateway as the agent's only tool surface, disabling OpenCode's built-in tools and project-provided tool configuration for task runs.
- Permit workspace reads through the gateway and mediate all file writes/deletes using phase-specific path rules.
- Require a machine-readable write-scope manifest in the generated specification and persist its validated paths when the PO approves that specification.
- Block implementation and phase advancement when the manifest is missing/invalid or the resulting changes violate scope.
- Keep shell execution, network tools, subagents, QA, GitHub, approvals, and releases unavailable to the agent.
- Add deterministic protocol, path traversal, symlink, scope, prompt-injection, and workflow tests.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `agentic-development`: mediate agent tools through a phase-aware gateway and scope all writes.
- `project-task-workflow`: require approved write scope and block phase transitions after violations.

## Impact

Affected areas include the OpenCode executor, a local MCP server and its executable entry point, specification generation and approval, task specification persistence, implementation scope validation, database migration, tests, and the two OpenSpec capabilities above. The gateway constrains agent tool calls; it does not sandbox arbitrary code run by QA or Docker, which remain separate trusted backend operations.
