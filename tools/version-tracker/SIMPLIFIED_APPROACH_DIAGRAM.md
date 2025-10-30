# Simplified Approach: Visual Comparison

## Before: Accumulated History (Confusing)

```
┌─────────────────────────────────────────────────────────────┐
│ Attempt 1                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (from original patch)                     │
│   • No error context                                       │
│                                                             │
│ Result: FAILED                                              │
│ Error: "patch failed to apply at line 42"                  │
│ Store: previousAttempts[0] = patch1                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Attempt 2                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (reused)                                  │
│   • Error from attempt 1                                   │
│   • Previous patch 1 ← STORED                              │
│                                                             │
│ Result: FAILED                                              │
│ Error: "build failed: undefined symbol"                    │
│ Store: previousAttempts[1] = patch2                        │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Attempt 3                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (reused)                                  │
│   • Error from attempt 2                                   │
│   • Previous patch 1 ← STALE, CONFUSING                    │
│   • Previous patch 2 ← STALE, CONFUSING                    │
│   • Error from attempt 1 ← STALE, CONFUSING                │
│                                                             │
│ Problem: Mixed signals, unclear what to fix                │
└─────────────────────────────────────────────────────────────┘
```

**Issues:**
- ❌ Shows old patches that didn't work
- ❌ Shows old errors that are no longer relevant
- ❌ Wastes tokens on historical data
- ❌ Confuses LLM with mixed signals

---

## After: Current Failure Only (Clear)

```
┌─────────────────────────────────────────────────────────────┐
│ Attempt 1                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (from original patch)                     │
│   • No error context                                       │
│                                                             │
│ Result: FAILED                                              │
│ Error: "patch failed to apply at line 42"                  │
│ Store: lastBuildError = "patch failed..."                  │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Attempt 2                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (reused)                                  │
│   • ⚠️ Current Error: "patch failed..." ← ONLY THIS        │
│                                                             │
│ Result: FAILED                                              │
│ Error: "build failed: undefined symbol"                    │
│ Update: lastBuildError = "build failed..."                 │
└─────────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────────┐
│ Attempt 3                                                   │
├─────────────────────────────────────────────────────────────┤
│ Context:                                                    │
│   • Base context (reused)                                  │
│   • ⚠️ Current Error: "build failed..." ← ONLY THIS        │
│   • 🤔 Reflection: Analyze THIS error                      │
│                                                             │
│ Clear: Focus on fixing the current error                   │
└─────────────────────────────────────────────────────────────┘
```

**Benefits:**
- ✅ Shows only current error
- ✅ No historical confusion
- ✅ Saves tokens
- ✅ Clear signal to LLM

---

## Prompt Comparison

### Before (Attempt 3)

```markdown
## Previous Attempt #2 Failed
Error: build failed: undefined symbol

## Previous Attempt #1 Failed  
Error: patch failed to apply at line 42

## Previous Patches Tried
### Attempt 1:
[full patch 1 - 200 lines]

### Attempt 2:
[full patch 2 - 200 lines]

Total: ~400 lines of historical data
```

**Token Usage**: ~1600 tokens (400 lines × 4 chars/token)

### After (Attempt 3)

```markdown
## ⚠️ Attempt #2 Failed - Current Error

**Error Message:**
build failed: undefined symbol

**What you need to fix:**
Analyze the error message above to understand what went wrong in the last attempt.

## 🤔 Reflection Required (Attempt #3)

This is your 3rd attempt. Before providing the fix, first explain:
1. What specific error occurred in the last attempt
2. Why that error happened
3. What specific changes you'll make to fix it

Total: ~20 lines of focused context
```

**Token Usage**: ~80 tokens (20 lines × 4 chars/token)

**Savings**: ~1520 tokens per attempt (95% reduction in error context)

---

## State Management

### Before: Accumulate Everything

```go
type PatchContext struct {
    FailedHunks      []FailedHunk
    BuildError       string          // Current error
    PreviousAttempts []string        // ALL previous patches
    // ... other fields
}

// In loop
previousAttempts = append(previousAttempts, fix.Patch)
ctx.PreviousAttempts = previousAttempts
```

**State Growth**: O(n) where n = number of attempts
- Attempt 1: 0 patches stored
- Attempt 2: 1 patch stored
- Attempt 3: 2 patches stored
- Attempt N: N-1 patches stored

### After: Keep Only Current

```go
type PatchContext struct {
    FailedHunks []FailedHunk
    BuildError  string          // ONLY current error
    // PreviousAttempts removed
    // ... other fields
}

// In loop
lastBuildError = err.Error()
ctx.BuildError = lastBuildError
```

**State Growth**: O(1) - constant
- Attempt 1: 1 error stored
- Attempt 2: 1 error stored (replaced)
- Attempt 3: 1 error stored (replaced)
- Attempt N: 1 error stored (replaced)

---

## Example: Real Scenario

### Scenario: 3 Attempts to Fix a Patch

**Attempt 1**: Patch fails to apply
- Error: "Hunk #1 FAILED at line 42"
- LLM generates fix
- Fix fails to apply

**Attempt 2**: Fix fails to build
- Error: "undefined: NewClient"
- LLM generates new fix
- Fix fails to build

**Attempt 3**: Fix has wrong logic
- Error: "test failed: expected 5, got 3"
- LLM needs to analyze and fix

### Before: Confusing Prompt (Attempt 3)

```
## Previous Attempt #2 Failed
Error: undefined: NewClient
[patch 2]

## Previous Attempt #1 Failed
Error: Hunk #1 FAILED at line 42
[patch 1]
```

**LLM sees**: 3 different errors, 2 old patches
**LLM thinks**: "Which error should I fix? What went wrong?"
**Result**: Confused, may repeat mistakes

### After: Clear Prompt (Attempt 3)

```
## ⚠️ Attempt #2 Failed - Current Error
Error: undefined: NewClient

## 🤔 Reflection Required (Attempt #3)
This is your 3rd attempt. Before providing the fix, first explain:
1. What specific error occurred in the last attempt
   → "undefined: NewClient"
2. Why that error happened
   → "Missing import or wrong package"
3. What specific changes you'll make to fix it
   → "Add import for client package"
```

**LLM sees**: 1 current error, clear instructions
**LLM thinks**: "I need to fix the NewClient error"
**Result**: Focused, actionable fix

---

## Summary

| Aspect | Before | After |
|--------|--------|-------|
| **Context** | Accumulated history | Current failure only |
| **Token Usage** | High (~1600 tokens) | Low (~80 tokens) |
| **Clarity** | Confusing | Clear |
| **State Growth** | O(n) | O(1) |
| **LLM Focus** | Scattered | Focused |
| **Success Rate** | Lower (confused) | Higher (clear) |

The simplified approach provides **clearer signals** to the LLM by showing only what matters: the current failure that needs to be fixed.
