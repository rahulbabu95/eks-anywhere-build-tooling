# Prompt Improvements Implemented

## Summary

Based on the analysis of failed attempts and your guidance, I've implemented comprehensive prompt improvements to eliminate bias, remove bad examples, and provide actual error context.

## Changes Made

### 1. Removed "Original Complete Patch" Section ✅

**Why**: This section was causing catastrophic overfitting. The LLM was copying truncated hashes and wrong line numbers from this example.

**Before**:
```markdown
## Original Complete Patch
```diff
@@ -933,8 +933,8 @@ github.com/.../gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wB  ← TRUNCATED
```

**After**: Completely removed. We already show the intent and failed hunks - this was redundant and harmful.

### 2. Removed Biased/Specific Examples ✅

**Why**: Examples mentioning specific file types (go.sum, go.mod) or specific issues (hash truncation) were causing overfitting to this one test case.

**Removed**:
- Specific mentions of "go.sum" and "go.mod" in instructions
- Example showing "package v1.2.8 h1:ABC..." (too specific)
- Warnings about "don't prune the hash" (too specific to this case)

**Replaced with**: Generic, reusable instructions that work for any patch type.

### 3. Added Actual Error Messages ✅

**Why**: The LLM was guessing why attempts failed. Now it sees the real git apply error.

**Before**:
```markdown
## Previous Attempt #1
You tried this fix, but it failed validation:
[patch content]

Build error:
[generic error]
```

**After**:
```markdown
## Previous Attempt #1 Failed

**Error Message:**
```
error: patch failed: go.sum:933
error: go.sum: patch does not apply
```

**What you need to fix:**
Analyze the error message above to understand what went wrong.
The error tells you exactly which file and line failed to apply.
Use the 'Expected vs Actual' sections to see what needs to change.
```

**Key improvement**: We bubble up the ACTUAL git apply error, not assumptions.

### 4. Removed Misleading "Success" Classification ✅

**Why**: We were telling the LLM that files with offset hunks "succeeded" and should be kept unchanged. This was wrong.

**Before**:
```markdown
- FAILED files (FIX these): go.mod
- SUCCESSFUL files (keep UNCHANGED): go.sum
```

**After**: Removed this classification entirely. We only show failed hunks that need fixing.

### 5. Simplified Task Instructions ✅

**Why**: Instructions were too verbose and contradictory.

**Before** (verbose, 15+ lines):
```markdown
## Task
Generate a corrected patch that:
1. Preserves the exact metadata...
2. Includes changes for ALL files in the original patch:
   - FAILED files (FIX these): ...
   - SUCCESSFUL files (keep UNCHANGED): ...
   CRITICAL: For successful files, copy the changes EXACTLY...
3. Uses RELATIVE file paths...
4. Fixes the failed hunks...
5. Will compile successfully...

CRITICAL UNDERSTANDING:
The original patch was created against an OLD version...
[10 more lines of explanation]

YOUR TASK:
1. Understand the INTENT...
2. Find the SEMANTIC LOCATION...
[15 more lines]

EXAMPLE:
[Specific example that causes overfitting]
```

**After** (concise, clear):
```markdown
## Task
Generate a corrected patch that:
1. Preserves the exact metadata (From, Date, Subject) from the original patch
2. Fixes ALL failed hunks shown above to apply cleanly to the CURRENT file state
3. Uses RELATIVE file paths NOT absolute paths
4. Will compile successfully

## How to Generate the Fix

**Step 1: Understand the Intent**
Look at 'What the patch tried to do' to understand the semantic change being made.

**Step 2: Use Current File State**
The 'Expected vs Actual' sections show you:
- What the original patch expected (OLD version)
- What's actually in the file NOW (NEW version)
- The specific differences between them

You MUST use the ACTUAL CURRENT content as your starting point, not the expected content.

**Step 3: Find the Semantic Location**
Find where in the CURRENT file the change should be applied:
- Use the 'Current file content' section to see the broader context
- Match based on semantic meaning (package names, function names, etc.)
- Don't rely on line numbers from the original patch - they may have shifted

**Step 4: Generate the Patch**
Create a patch that:
- Uses context lines from the CURRENT file (complete, not truncated)
- Uses CURRENT line numbers
- Makes the SAME semantic change as the original patch intended
- Preserves exact formatting and whitespace from the current file
```

### 6. Removed Assumptive "Why it failed" Section ✅

**Why**: We were making assumptions about why patches failed instead of letting the error message speak for itself.

**Before**:
```markdown
### Why it failed:
The patch expects specific content/formatting that differs from the current file state.
See the differences above for details.
```

**After**: Removed. The actual git apply error message tells the real story.

### 7. Cleaned Up Warning Language ✅

**Why**: Too many ⚠️ symbols and shouty language was creating noise.

**Before**:
```markdown
⚠️  USE THIS AS YOUR 'BEFORE' STATE - NOT THE EXPECTED CONTEXT ABOVE!
⚠️  Your patch MUST use the NEW version content (actual context above) as the starting point!
```

**After**:
```markdown
**What's actually in the file now (CURRENT version):**
```

Cleaner, more professional, less noisy.

### 8. Removed extractSuccessfulFiles Function ✅

**Why**: No longer needed since we're not classifying files as "successful" vs "failed".

**Deleted**: 25 lines of unused code.

---

## Key Principles Applied

### 1. Generic, Not Specific
- No mentions of specific file types (go.sum, go.mod)
- No examples tied to one test case
- Instructions work for any patch type

### 2. Facts, Not Assumptions
- Show actual error messages from git apply
- Don't invent reasons for failures
- Let the LLM analyze real data

### 3. Concise, Not Verbose
- Removed redundant sections
- Simplified instructions
- Focused on essential information

### 4. Context, Not Examples
- Provide rich context (Expected vs Actual)
- Remove biased examples that cause overfitting
- Let the LLM learn from the actual data

---

## What the Prompt Looks Like Now

### Structure:
```
## Project: [name]

## Original Patch Metadata
From: ...
Date: ...
Subject: ...

## Original Patch Intent
[Brief description]

## Failed Hunk #1 in [file]

### What the patch tried to do:
[Diff from .rej file]

### Expected vs Actual File State:
**What the original patch expected (from OLD version):**
[Context from .rej]

**What's actually in the file now (CURRENT version):**
[Actual file content]

**Differences:**
- [Specific differences]

### Current file content (around line X):
[Broader context]

### Instructions:
- Use the ACTUAL CURRENT content shown above as your starting point
- Match the exact formatting and whitespace from the current file
- Use current line numbers, not the original patch's line numbers

---

[Repeat for each failed hunk]

## Previous Attempt #N Failed  [Only if attempt > 1]

**Error Message:**
```
[Actual git apply error]
```

**What you need to fix:**
Analyze the error message above to understand what went wrong.
The error tells you exactly which file and line failed to apply.
Use the 'Expected vs Actual' sections to see what needs to change.

## Reflection Required  [Only if attempt >= 3]
Before providing the fix, first explain:
1. Why the previous attempts failed
2. What needs to change in this attempt
3. The specific lines that need modification

Then provide the corrected patch.

## Task
Generate a corrected patch that:
1. Preserves the exact metadata (From, Date, Subject) from the original patch
2. Fixes ALL failed hunks shown above to apply cleanly to the CURRENT file state
3. Uses RELATIVE file paths NOT absolute paths
4. Will compile successfully

## How to Generate the Fix

**Step 1: Understand the Intent**
...

**Step 2: Use Current File State**
...

**Step 3: Find the Semantic Location**
...

**Step 4: Generate the Patch**
...

## Output Format
[Simple format example]
```

---

## Expected Improvements

### 1. No More Overfitting
- LLM won't copy truncated hashes from bad examples
- LLM won't use wrong line numbers from "Original Complete Patch"
- Each test case is evaluated on its own merits

### 2. Better Learning Between Attempts
- LLM sees actual error messages
- LLM knows exactly which line failed
- LLM can analyze the specific mismatch

### 3. More Generic Solution
- Works for any file type (not just go.sum/go.mod)
- Works for any kind of patch conflict
- Instructions are reusable across projects

### 4. Clearer Instructions
- Less noise, more signal
- Step-by-step guidance
- Focus on the essential task

---

## Testing

To test these improvements:

```bash
# Rebuild (already done)
cd tools/version-tracker && make build

# Run fresh test
cd ../../test/eks-anywhere-build-tooling
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-$(date +%Y%m%d-%H%M%S).log
```

### What to Check:

1. **Prompt file** (`/tmp/llm-prompt-attempt-1.txt`):
   - ✅ No "Original Complete Patch" section
   - ✅ No specific go.sum/go.mod examples
   - ✅ Clean, concise instructions
   - ✅ Expected vs Actual sections present

2. **Attempt 2 prompt** (`/tmp/llm-prompt-attempt-2.txt`):
   - ✅ Shows actual git apply error
   - ✅ "Previous Attempt #1 Failed" section with error details
   - ✅ Different from attempt 1 (has new context)

3. **Response files**:
   - ✅ LLM generates different patches on retry
   - ✅ LLM uses complete hashes (not truncated)
   - ✅ LLM uses current line numbers

---

## Summary

These changes transform the prompt from:
- ❌ Biased, overfitting, assumptive
- ❌ Verbose, contradictory, noisy
- ❌ Specific to one test case

To:
- ✅ Generic, reusable, data-driven
- ✅ Concise, clear, focused
- ✅ Works for any patch type

The key insight: **Provide rich context, not bad examples. Show actual errors, not assumptions.**
