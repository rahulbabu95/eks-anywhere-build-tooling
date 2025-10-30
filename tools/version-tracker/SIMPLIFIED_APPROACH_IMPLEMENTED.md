# Simplified Approach: Show Only Current Failure Context

## Problem
Previously, the LLM prompt was showing accumulated history of previous attempts, which caused:
1. **Context pollution**: Stale failure information from earlier attempts
2. **Confusion**: Mixed signals about what actually failed
3. **Token waste**: Storing and sending old patch attempts that aren't helpful

## Solution: Simplified Approach
Show only the **current failure context** for each consecutive patch application attempt.

### Key Changes

#### 1. Removed Previous Attempts History
**Before:**
```go
var previousAttempts []string
var lastBuildError string

// Store both error and patch
previousAttempts = append(previousAttempts, fix.Patch)
ctx.PreviousAttempts = previousAttempts
```

**After:**
```go
var lastBuildError string

// Store ONLY the current error
lastBuildError = err.Error()
// Note: PreviousAttempts is intentionally NOT populated
```

#### 2. Updated Prompt to Show Only Current Failure
**Before:**
```
## Previous Attempt #2 Failed

**Error Message:**
[error from attempt 2]

**Previous patches tried:**
[patch from attempt 1]
[patch from attempt 2]
```

**After:**
```
## ⚠️ Attempt #2 Failed - Current Error

**Error Message:**
[error from attempt 2 ONLY]

**What you need to fix:**
Analyze the error message above to understand what went wrong in the last attempt.
```

#### 3. Enhanced Reflection for Later Attempts
For attempt 3+, we now ask the LLM to:
1. Analyze the **specific error** from the last attempt
2. Explain **why** that error happened (using Expected vs Actual)
3. Describe **what specific changes** will fix it

This focuses the LLM on the current problem, not historical context.

### Benefits

1. **Clearer Context**: LLM sees only relevant information
   - Original patch status (extracted once)
   - Current failure error (from last attempt)
   - No stale information

2. **Reduced Token Usage**: 
   - No storing/sending old patch attempts
   - Smaller prompts = faster responses
   - More budget for actual context

3. **Better Debugging**:
   - Each attempt focuses on fixing the current error
   - No confusion from mixed signals
   - Easier to understand what went wrong

4. **Simpler Code**:
   - Removed `previousAttempts` array
   - Removed logic to accumulate history
   - Single source of truth: `lastBuildError`

### Implementation Details

#### Files Modified
1. `pkg/commands/fixpatches/llm.go`:
   - Updated `BuildPrompt()` to show only current failure
   - Added `ordinal()` helper for better messaging
   - Removed previous attempts section

2. `pkg/commands/fixpatches/fixpatches.go`:
   - Removed `previousAttempts` variable
   - Simplified error tracking to just `lastBuildError`
   - Updated comments to reflect simplified approach

#### Context Flow (Simplified)
```
Attempt 1:
  - Base context (from original patch)
  - No error context (first try)

Attempt 2:
  - Base context (reused)
  - Error from attempt 1 ONLY

Attempt 3:
  - Base context (reused)
  - Error from attempt 2 ONLY
  - Reflection prompt

Attempt N:
  - Base context (reused)
  - Error from attempt N-1 ONLY
  - Reflection prompt
```

### Testing Recommendations

1. **Single Attempt Success**: Verify first attempt works as before
2. **Multi-Attempt Scenario**: Test that subsequent attempts show only current error
3. **Token Usage**: Verify reduced token consumption
4. **Error Clarity**: Check that error messages are clear and actionable

### Example Prompt Comparison

#### Before (Attempt 3)
```
## Previous Attempt #2 Failed
Error: patch failed to apply
[Previous patch 1]
[Previous patch 2]

## Previous Attempt #1 Failed  
Error: build failed
[Previous patch 1]
```
**Problem**: Confusing, redundant, token-heavy

#### After (Attempt 3)
```
## ⚠️ Attempt #2 Failed - Current Error
Error: build failed at line 42

## 🤔 Reflection Required (Attempt #3)
This is your 3rd attempt. Before providing the fix, first explain:
1. What specific error occurred in the last attempt
2. Why that error happened
3. What specific changes you'll make to fix it
```
**Benefit**: Clear, focused, actionable

## Conclusion

This simplified approach eliminates context pollution by showing only the current failure state. The LLM gets:
- ✅ Original patch context (pristine, extracted once)
- ✅ Current error (from last attempt only)
- ❌ No historical attempts (removed)

This makes debugging easier, reduces token usage, and provides clearer signals to the LLM about what needs to be fixed.
