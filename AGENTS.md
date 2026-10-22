# GolgiMed Mimic agent instructions

## Mandatory Incremental Delivery: Commit, Push, and Draft PR

**Core principle:** Work must be published incrementally. Do not keep completed, coherent work trapped in a local checkout until the entire task is finished.

### 1. Establish Remote Traceability Early

At task start:

1. Identify the issue or task, repository, base branch, and intended scope.
2. Create or select a dedicated task branch in an isolated checkout.
3. Confirm the remote and branch permissions.
4. Preserve any existing uncommitted or untracked work; never reuse another task's branch without explicit coordination.

### 2. Commit at Coherent Checkpoints

Whenever a coherent, reviewable unit of work is complete:

1. Run the relevant focused tests and checks.
2. Review the staged diff for unrelated changes, secrets, generated artifacts, and accidental deletions.
3. Create a narrowly scoped commit.
4. Push the task branch to the authorized remote.
5. Verify the remote branch points to the expected commit.

Do not wait for the entire task to finish. Do not create meaningless commits merely to increase their number.

### 3. Open a Draft PR Early

Open a Draft PR as soon as the first coherent checkpoint has been pushed and the branch is ready for review.

The Draft PR should include:

- Task or issue reference, if one exists.
- Problem statement and intended behavior.
- Implemented changes and current validation status.
- Known gaps, incomplete work, and risks.
- Relevant test evidence.

A Draft PR may contain incomplete work. Its description must clearly identify what is implemented, what remains, and which checks have or have not passed.

### 4. Keep the Draft PR Updated

For each subsequent coherent checkpoint:

1. Commit the changes.
2. Push the updated branch.
3. Verify the remote SHA.
4. Update the Draft PR description and evidence when its scope or validation status changes.

Prefer pushing updates to the same task branch and PR instead of creating a new PR for every checkpoint.

### 5. Handle Blockers Without Losing Progress

If a task becomes blocked:

- Commit and push any coherent, safe, reviewable work that is ready for delivery.
- Keep the PR in Draft.
- Document the exact blocker and next action.
- Continue independent work where safe.
- Never bypass authorization, branch protections, required checks, or review requirements to force publication.

If publication is prohibited or unavailable, preserve the local checkpoint and report the exact limitation. Do not claim that remote delivery occurred.

### 6. Completion and Merge

A task is not considered remotely deliverable merely because its local tests pass.

Track these states separately:

- Local implementation complete.
- Local validation passed.
- Commit created.
- Branch pushed and remote SHA verified.
- Draft PR opened and verified.
- Review and required checks completed.
- Merge authorized and verified.

Convert a Draft PR to ready for review only when the change meets the repository's review criteria. Merge only with the required authorization and protections intact.

### 7. Required Status Report

At every meaningful checkpoint, report:

- Branch and base SHA.
- New commit SHA.
- Remote push status and verified remote SHA.
- Draft PR URL and status, if created.
- Tests executed and results.
- Remaining work and blockers.

**Default behavior:** deliver incrementally and visibly. Do not let finished work accumulate locally without a concrete reason.
