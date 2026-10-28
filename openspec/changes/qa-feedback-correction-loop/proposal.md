# Proposal

## Why

QA can reject an implementation with a comment and evidence, but the return path to development does not start a correction run or pass that feedback to the implementation agent. The rejected task is left waiting, so the defect and review recommendations do not become an actionable development cycle.

## What Changes

- Preserve QA rejection comments and evidence as the correction instructions associated with the task.
- Make the developer's “Iniciar correção” action start a new implementation run and include the latest QA recommendations in its prompt.
- Keep the task ineligible for PR until the correction returns to QA and receives approval.
- Record the correction run and its outcome in the existing task history and implementation records.

## Capabilities

### New Capabilities

### Modified Capabilities

- `project-task-workflow`: complete the handoff from QA rejection to an implementation run informed by QA feedback, while preserving the QA approval gate.

## Impact

Touches task transition handling and implementation-run creation in the backend, the prompt context passed to the implementation agent, and the task UI's correction status and progress. Reuses task events, implementation records, and the existing role permissions; no new service or database table is expected.
