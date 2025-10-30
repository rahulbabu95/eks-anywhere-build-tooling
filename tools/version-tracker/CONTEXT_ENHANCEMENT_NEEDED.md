# Context Enhancement for LLM Patch Fixing

## Problem Identified

The patch fails because of whitespace/formatting differences between what the patch expects and what's actually in the file.

### Example from fluxcd/source-controller

**Patch expects:**
```go
replace github.com/opencontainers/go-digest => ... v1.0.1-0.20220411205349-bde1400a84be

require (
```
(Note: blank line between replace and require)

**Actual file has:**
```go
replace github.com/opencontainers/go-digest => ... v1.0.1-0.20220411205349-bde1400a84be
require (
```
(Note: NO blank line)

**What needs to happen:**
The LLM needs to generate a patch that:
1. Matches the ACTUAL current state (no blank line)
2. Adds the new replace statement
3. Maintains the current formatting

## Current Context Provided to LLM

From `.rej` file:
- The failed hunk showing what the patch tried to do
- ±50 lines of context around the failure point

**What's Missing:**
- Clear indication of EXACT current state
- Explicit comparison: "patch expects X, but file has Y"
- Guidance on how to adapt the patch

## Recommended Enhancements

### 1. Add "Current vs Expected" Section to Prompt

```
## Current File State vs Patch Expectations

For each failed hunk, show:
- What the patch is looking for (the context lines)
- What's actually in the file at that location
- The specific differences (whitespace, line numbers, content)
```

### 2. Enhanced Context Extraction

Instead of just showing ±50 lines, show:
- The EXACT lines the patch is trying to match
- The ACTUAL lines at that location in the current file
- A diff between expected and actual

### 3. Clearer Instructions

Add to prompt:
```
IMPORTANT: The patch may fail because:
1. Line numbers have shifted (file has grown/shrunk)
2. Whitespace differences (blank lines added/removed)
3. Content has changed slightly (version numbers, formatting)

Your task is to:
1. Identify what the patch is trying to achieve (the intent)
2. Find where that change should go in the CURRENT file state
3. Generate a patch that applies to the CURRENT state, not the expected state
```

### 4. Show Actual File Snippet

Add to context:
```go
## Actual Current File Content (go.mod lines 8-12)
```
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be
require (
        cloud.google.com/go/compute/metadata v0.9.0
```

## What the Patch Expects (from .rej file)
```
// xref: https://github.com/opencontainers/go-digest/pull/66
replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be

require (
```

## Difference
- Current file: NO blank line between replace and require
- Patch expects: blank line between replace and require

## Required Fix
Add the timestamp-authority replace WITHOUT assuming a blank line exists.
```

## Implementation Plan

### Step 1: Enhance extractFileContext()

Currently shows ±50 lines. Enhance to:
1. Extract the exact lines the patch is trying to match
2. Show them side-by-side with what's actually there
3. Highlight differences

### Step 2: Add "Expected vs Actual" to Prompt

In `BuildPrompt()`, add a new section:
```go
prompt.WriteString("## Expected vs Actual File State\n\n")
for _, hunk := range ctx.FailedHunks {
    prompt.WriteString(fmt.Sprintf("### File: %s\n", hunk.FilePath))
    prompt.WriteString("**What the patch expects to find:**\n")
    prompt.WriteString("```\n")
    // Extract context lines from the hunk
    prompt.WriteString("```\n\n")
    
    prompt.WriteString("**What's actually in the file:**\n")
    prompt.WriteString("```\n")
    // Show actual file content
    prompt.WriteString("```\n\n")
    
    prompt.WriteString("**Key differences:**\n")
    // List specific differences
    prompt.WriteString("\n")
}
```

### Step 3: Update Task Instructions

Make it crystal clear:
```
Your task is to generate a patch that:
1. Applies to the ACTUAL current file state (not what the original patch expected)
2. Achieves the same intent as the original patch
3. Uses the correct line numbers for the CURRENT file
4. Matches the current file's formatting and whitespace
```

## Expected Outcome

With these enhancements, the LLM should generate:

```diff
diff --git a/go.mod b/go.mod
index 21c15753..79d1b5c8 100644
--- a/go.mod
+++ b/go.mod
@@ -8,6 +8,8 @@ replace github.com/fluxcd/source-controller/api => ./api
 // xref: https://github.com/opencontainers/go-digest/pull/66
 replace github.com/opencontainers/go-digest => github.com/opencontainers/go-digest v1.0.1-0.20220411205349-bde1400a84be
+
+replace github.com/sigstore/timestamp-authority => github.com/sigstore/timestamp-authority v1.2.0
 require (
        cloud.google.com/go/compute/metadata v0.9.0
```

Note: This adds BOTH blank lines (one before the new replace, matching the pattern) and adapts to the current state.

## Next Steps

1. Implement enhanced context extraction
2. Update prompt with "Expected vs Actual" section
3. Test with the fluxcd/source-controller patch
4. Verify the LLM can now generate correct patches

## Why This Matters

Without understanding the exact differences between expected and actual state, the LLM is essentially guessing. By providing:
- Explicit comparison
- Clear differences
- Current actual state

The LLM can make informed decisions about how to adapt the patch.
