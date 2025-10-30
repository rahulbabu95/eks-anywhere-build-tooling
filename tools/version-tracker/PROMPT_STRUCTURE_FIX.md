# Prompt Structure Fix - Critical Context Positioning

## Problem Identified

You correctly identified that the LLM was ignoring critical error information:

**Before (Bad Structure)**:
```
Line 1-950:   Project info, failed hunks, file context
Line 952-970: ⚠️ CRITICAL ERROR: "corrupt patch at line 276"
Line 970-2600: 📄 HUGE ORIGINAL PATCH (1700+ lines)
Line 2600+:   Task instructions
```

**Result**: LLM's attention gets buried in the 1700-line patch. It never focuses on the "corrupt patch" error.

## Root Cause

The prompt structure had:
1. Error message early (line ~950)
2. Immediately followed by massive original patch (1700+ lines)
3. Task instructions at the very end

LLMs have **recency bias** - they pay more attention to information that appears:
- At the beginning (primacy effect)
- At the end (recency effect)
- Right before the task

The error was in the "middle" and got buried.

## The Fix

**After (Good Structure)**:
```
Line 1-900:   Project info, failed hunks, file context
Line 900-2500: 📄 Original patch (for reference)
Line 2500-2550: 🚨 CRITICAL ERROR: "corrupt patch at line 276"
Line 2550+:   Task instructions
```

**Result**: Error is the LAST thing LLM sees before the task, ensuring maximum attention.

## Changes Made

### 1. Moved Error Section
**From**: After file context, before original patch
**To**: After original patch, right before task instructions

### 2. Enhanced Error Visibility
```go
// BEFORE (buried)
## ⚠️ Attempt #1 Failed - Current Error
**Error Message:**
```
error: corrupt patch at line 276
```

// AFTER (prominent)
---
# 🚨 CRITICAL: Attempt #1 Failed With This Error

**Your previous patch failed to apply with this error:**
```
error: corrupt patch at line 276
```

**🎯 Your primary goal:**
Fix the SPECIFIC error shown above...
---
```

### 3. Added Error Interpretation Guide
```
**Common causes of these errors:**
- 'corrupt patch at line X': Patch format is malformed
- 'patch fragment without header': Missing 'diff --git' headers
- 'does not apply': Line numbers don't match
```

This helps the LLM understand what the error means and how to fix it.

## Why This Works

### Attention Mechanisms
LLMs use attention to focus on relevant parts of the prompt. By placing the error:
1. **After** the reference material (original patch)
2. **Before** the task instructions
3. **With visual markers** (🚨, ---, bold)

We ensure it gets maximum attention weight.

### Information Flow
```
Context (what happened) 
  ↓
Reference (original patch)
  ↓
🚨 ERROR (what went wrong) ← FOCUS HERE
  ↓
Task (what to do)
```

The error is now in the optimal position: right before the action.

## Expected Impact

### Before Fix
- Attempt 1: Generates patch, fails with "corrupt patch at line 276"
- Attempt 2: Ignores error, makes same semantic fix, fails again
- Attempt 3: Still ignoring error, fails again

### After Fix
- Attempt 1: Generates patch, fails with "corrupt patch at line 276"
- Attempt 2: **Sees error prominently**, understands it's a format issue, fixes it
- Success!

## Testing

Next test run should show in the LLM response:
```
## Analysis

The previous attempt failed with "corrupt patch at line 276". 
This indicates a patch format issue...
```

Instead of:
```
## Analysis

The patch is trying to remove all cloud providers except CAPI...
(completely ignoring the error)
```

## Additional Benefits

1. **Clearer error interpretation**: Added guide for common errors
2. **Better reflection prompts**: Updated attempt 3+ to reference "error above"
3. **Visual separation**: Used `---` and `#` headers for prominence
4. **Goal clarity**: Explicit "Your primary goal: Fix the SPECIFIC error"

## Code Changes

**File**: `tools/version-tracker/pkg/commands/fixpatches/llm.go`

- Moved error section from line ~498 to after original patch (~565)
- Enhanced error formatting with visual markers
- Added error interpretation guide
- Updated reflection prompts to reference error location
