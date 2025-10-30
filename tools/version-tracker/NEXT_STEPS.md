# Next Steps: Context Enhancement Implementation

## Current Situation

✅ **Analysis Complete**: No open PRs currently have patch application failures
✅ **Test Candidate Identified**: fluxcd/source-controller (go.mod/go.sum whitespace issue)
✅ **Root Cause Understood**: LLM doesn't see the exact difference between expected vs actual file state

## Decision: Proceed with Context Enhancement

Since there are no active failing PRs to test against, we'll implement the context enhancement to handle the fluxcd/source-controller case, which represents a common failure pattern (whitespace/formatting differences).

---

## Implementation Plan

### Task 3.1: Enhance Context Extraction (NEXT)

**File**: `tools/version-tracker/pkg/commands/fixpatches/context.go`

**What to implement:**

1. **Add "Expected vs Actual" comparison**
   - Extract the exact lines the patch expects to find (from .rej file context lines)
   - Read the actual current file content at that location
   - Compare and highlight differences (whitespace, blank lines, content)

2. **Enhance PatchContext struct**
   ```go
   type FailedHunk struct {
       FilePath        string
       HunkHeader      string
       FailedContent   string
       ExpectedContext []string  // NEW: What patch expects to find
       ActualContext   []string  // NEW: What's actually in the file
       Differences     []string  // NEW: Specific differences
   }
   ```

3. **Implement comparison logic**
   - Parse .rej file to extract expected context lines (lines starting with space or -)
   - Read actual file at the target location
   - Identify differences:
     - Blank line mismatches
     - Whitespace differences
     - Content changes
     - Line number shifts

### Task 4.2: Enhance Prompt Building (AFTER 3.1)

**File**: `tools/version-tracker/pkg/commands/fixpatches/llm.go`

**What to implement:**

1. **Add "Expected vs Actual" section to prompt**
   ```
   ## Expected vs Actual File State
   
   For each failed hunk:
   - Show what the patch expects
   - Show what's actually there
   - Highlight key differences
   ```

2. **Add clearer instructions**
   ```
   IMPORTANT: Generate a patch that:
   1. Applies to the ACTUAL current file state
   2. Achieves the same intent as the original patch
   3. Matches current formatting and whitespace
   ```

---

## Test Plan

### Manual Test with fluxcd/source-controller

```bash
# Create a test scenario
cd /Users/rahulgab/Desktop/work/1-30/eks-anywhere-build-tooling/test/eks-anywhere-build-tooling

# Simulate patch failure by trying to apply the patch
cd projects/fluxcd/source-controller
git apply --reject patches/0001-*.patch

# This should create .rej files showing the whitespace mismatch

# Then test our enhanced fix-patches command
cd /Users/rahulgab/Desktop/work/1-30/eks-anywhere-build-tooling/test/eks-anywhere-build-tooling
../bin/version-tracker fix-patches \
  --project fluxcd/source-controller \
  --max-attempts 1 \
  --verbosity 6
```

### Success Criteria

✅ Context extraction identifies the blank line difference
✅ Prompt clearly shows "expected vs actual"
✅ LLM generates patch that matches actual file state
✅ Patch applies cleanly
✅ go.mod has correct formatting

---

## Implementation Order

1. **Task 3.1**: Enhance context extraction
   - Add ExpectedContext, ActualContext, Differences to FailedHunk
   - Implement comparison logic
   - Extract differences (whitespace, blank lines, etc.)

2. **Task 4.2**: Update prompt building
   - Add "Expected vs Actual" section
   - Include specific differences
   - Add clearer instructions

3. **Test**: Run manual test with fluxcd/source-controller

4. **Iterate**: Refine based on results

---

## Why This Approach

1. **Addresses root cause**: The LLM can't fix what it can't see
2. **Generalizable**: Will help with many patch failure types
3. **Low risk**: Only enhances context, doesn't change core logic
4. **Testable**: Can validate with known failure case

---

## Expected Outcome

After implementation, the LLM should be able to:
- See the exact whitespace difference
- Understand the current file state
- Generate a patch that matches reality
- Handle similar formatting issues in other projects

---

## Ready to Start?

Next command:
```
Start implementing Task 3.1: Enhance context extraction in context.go
```

This will add the "Expected vs Actual" comparison logic that the LLM needs to generate correct patches.
