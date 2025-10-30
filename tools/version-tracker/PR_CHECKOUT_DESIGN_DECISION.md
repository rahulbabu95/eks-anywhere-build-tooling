# PR Checkout - Design Decision

## Decision: Lambda Handler Handles Checkout

**Date**: 2025-10-16

## Context

During testing, we discovered that the `fix-patches` tool doesn't checkout PR branches - it assumes you're already on the correct branch. This raised the question: should the tool or the Lambda handler handle PR checkout?

## Decision

**Lambda handler will handle PR checkout.** The `fix-patches` tool remains focused on its core responsibility: fixing patches in the current working directory.

## Rationale

### Why Lambda Handler?

1. **Clean separation of concerns**
   - Tool = Pure patch-fixing business logic
   - Lambda = Orchestration (clone, checkout, run tool, commit, push)

2. **Tool portability**
   - Can be run in any environment (local, CI, Lambda, CodeBuild)
   - No dependencies on GitHub, git operations, or environment setup
   - Just point it at a directory with patches

3. **Easier testing**
   - Tool can be tested without mocking git operations
   - Lambda orchestration can be tested separately

4. **Lambda is stateless anyway**
   - Lambda starts fresh each time
   - Must clone/checkout regardless
   - Natural place for this logic

5. **Flexibility**
   - Lambda can handle pre/post checkout operations
   - Can retry, cleanup, handle errors at orchestration level

### Why NOT in the Tool?

1. **Avoids coupling** - Tool doesn't need to know about GitHub, tokens, repo URLs
2. **Keeps tool simple** - Single responsibility: fix patches
3. **Avoids state management** - Tool doesn't handle dirty repos, conflicts, etc.
4. **Maintains design consistency** - Follows existing patterns in the codebase

## Implementation

### For Production (Lambda)
Lambda handler (Task 12.1) will:
```python
1. Clone eks-anywhere-build-tooling repo
2. Checkout PR branch (git fetch + git checkout)
3. Run: version-tracker fix-patches --project <name> --pr <number>
4. Commit and push changes
5. Comment on PR
```

### For Manual Testing
Use the helper script:
```bash
cd test/eks-anywhere-build-tooling
../../tools/version-tracker/test-fix-patches.sh 4858 kubernetes/autoscaler
```

The script handles:
- Fetching the PR branch
- Checking it out
- Running fix-patches with correct parameters

## Trade-offs Considered

| Aspect | Lambda Handles | Tool Handles |
|--------|---------------|--------------|
| Separation of concerns | ✅ Clean | ❌ Mixed |
| Tool portability | ✅ High | ❌ Low |
| Testing complexity | ✅ Simple | ❌ Complex |
| Manual testing UX | ⚠️ Need helper script | ✅ One command |
| Lambda complexity | ⚠️ More logic | ✅ Simpler |
| Maintenance | ✅ Clear boundaries | ❌ Two concerns |

## Current Status

- ✅ Tool implementation complete (no checkout logic)
- ✅ Helper script created for manual testing
- ⏳ Lambda handler (Task 12.1) - not started
- ⏳ CDK infrastructure (Task 13) - not started

## Usage

### Manual Testing
```bash
# Option 1: Use helper script
cd test/eks-anywhere-build-tooling
../../tools/version-tracker/test-fix-patches.sh <PR_NUMBER> <PROJECT>

# Option 2: Manual checkout
git fetch origin pull/<PR>/head:test-pr-<PR>
git checkout test-pr-<PR>
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
  --project <PROJECT> \
  --pr <PR> \
  --max-attempts 3 \
  --verbosity 6
```

### Production (Lambda)
Lambda will handle everything automatically when triggered by EventBridge.

## Related Documents

- Design: `.kiro/specs/llm-patch-fixer/design.md`
- Tasks: `.kiro/specs/llm-patch-fixer/tasks.md` (Task 12.1)
- Helper script: `tools/version-tracker/test-fix-patches.sh`

## Conclusion

This decision maintains clean architecture, keeps the tool focused and portable, and aligns with the existing design. The minor inconvenience for manual testing is addressed by the helper script and is temporary until Lambda is implemented.
