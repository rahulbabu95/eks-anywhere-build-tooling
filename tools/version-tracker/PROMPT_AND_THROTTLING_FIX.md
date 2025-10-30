# Fix: LLM Prompt and Throttling Issues

## Problems Identified

### 1. LLM Only Fixing Partial Patch
The LLM was only generating fixes for the conflicting parts, not the complete patch:
- Original patch had changes to `go.mod` AND `go.sum`
- LLM output only had `go.mod` changes
- Missing the complete patch structure

### 2. Wrong File Paths in LLM Output
The LLM was generating patches with incorrect absolute paths:
```diff
diff --git a/projects/fluxcd/source-controller/source-controller/go.mod
```

Should be:
```diff
diff --git a/go.mod b/go.mod
```

This caused `git apply` to fail with "No such file or directory".

### 3. Throttling Errors
After 2-3 rapid API calls, Bedrock was returning:
```
ThrottlingException: Too many tokens, please wait before trying again.
```

The exponential backoff was too short (1s, 2s, 3s) for token-based throttling.

## Solutions

### 1. Improved Prompt - Include Complete Original Patch

Added the complete original patch to the prompt for reference:

```go
// Original patch for reference
prompt.WriteString("## Original Complete Patch\n")
prompt.WriteString("```diff\n")
prompt.WriteString(ctx.OriginalPatch)
prompt.WriteString("\n```\n\n")
```

### 2. Clearer Instructions

Updated task instructions to be explicit:

```
Generate a corrected patch that:
1. Preserves the exact metadata (From, Date, Subject) from the original patch
2. Includes ALL files from the original patch (not just the failed ones)
3. Uses RELATIVE file paths (e.g., 'go.mod', 'go.sum') NOT absolute paths
4. Fixes the failed hunks to apply cleanly to the current file state
5. Keeps successful hunks unchanged
6. Will compile successfully

CRITICAL: Output the COMPLETE corrected patch with ALL files, not just the conflicting parts.
```

### 3. Better Output Format Example

Provided a clear example showing multiple files:

```
Output format (unified diff with complete headers):
From <commit-hash> Mon Sep 17 00:00:00 2001
From: Author <email>
Date: ...
Subject: ...

---
 file1.ext | X +/-
 file2.ext | Y +/-
 N files changed, X insertions(+), Y deletions(-)

diff --git a/file1.ext b/file1.ext
...
```

### 4. Increased max_tokens

Changed from 4096 to 8192 to allow for complete patches:

```go
"max_tokens": 8192, // Increased to allow for complete patches
```

### 5. Better Exponential Backoff

Increased wait times for throttling:

**Before:**
```go
time.Sleep(time.Second * time.Duration(i+1)) // 1s, 2s, 3s
```

**After:**
```go
// Exponential backoff: 2s, 4s, 8s
waitTime := time.Duration(2<<uint(i)) * time.Second
logger.Info("Waiting before retry", "wait_seconds", waitTime.Seconds())
time.Sleep(waitTime)
```

This gives Bedrock more time to recover from token-based throttling.

## Expected Behavior After Fix

### LLM Output Should Now Include:

1. **Complete patch metadata**
   ```
   From f8d85ab... Mon Sep 17 00:00:00 2001
   From: Abhay Krishna Arunachalam <arnchlm@amazon.com>
   Date: Wed, 7 Feb 2024 22:30:29 -0800
   Subject: [PATCH] Replace timestamp-authority...
   ```

2. **All files from original patch**
   ```
   ---
    go.mod | 2 ++
    go.sum | 4 ++--
    2 files changed, 4 insertions(+), 2 deletions(-)
   ```

3. **Correct relative paths**
   ```diff
   diff --git a/go.mod b/go.mod
   diff --git a/go.sum b/go.sum
   ```

4. **Complete hunks for all files**
   - Fixed hunks for conflicting parts
   - Unchanged hunks for parts that applied cleanly

### Throttling Handling:

When throttled, you'll see:
```
Bedrock API call failed, retrying attempt=1
Waiting before retry wait_seconds=2
Bedrock API call failed, retrying attempt=2
Waiting before retry wait_seconds=4
Bedrock API call failed, retrying attempt=3
Waiting before retry wait_seconds=8
```

This gives Bedrock 14 seconds total to recover (2+4+8).

## Why These Changes Matter

### Before:
- ❌ LLM only fixed go.mod, ignored go.sum
- ❌ Wrong file paths caused apply failures
- ❌ Throttling killed the 3rd attempt
- ❌ No way to succeed even with retries

### After:
- ✅ LLM outputs complete patch with all files
- ✅ Correct relative paths
- ✅ Better throttling handling
- ✅ Higher chance of success

## Files Changed

- `tools/version-tracker/pkg/commands/fixpatches/llm.go`
  - Added complete original patch to prompt
  - Clarified instructions for complete output
  - Added explicit path format requirements
  - Increased max_tokens from 4096 to 8192
  - Improved exponential backoff (2s, 4s, 8s)

## Testing

After this fix, check the debug file:

```bash
cat projects/fluxcd/source-controller/.llm-patch-debug.txt
```

You should see:
1. ✅ Both `go.mod` and `go.sum` changes
2. ✅ Relative paths: `diff --git a/go.mod b/go.mod`
3. ✅ Complete patch structure
4. ✅ All metadata preserved

And in the logs:
```
Prompt built length=X estimated_tokens=Y
Waiting before retry wait_seconds=2 (if throttled)
Received response from Bedrock response_length=Z
```

## Additional Notes

### Token Limits

Claude Sonnet 4.5 has:
- **Input**: 200K tokens
- **Output**: 8K tokens (we set max_tokens=8192)

For this simple patch, we're well within limits. The throttling is about **rate limiting** (requests per second), not total tokens.

### Rate Limits

Bedrock has default rate limits:
- **Requests per minute**: Varies by model and region
- **Tokens per minute**: Varies by model and region

The exponential backoff helps us stay within these limits when making multiple rapid requests.
