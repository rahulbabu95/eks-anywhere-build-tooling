# Context Enhancement and Debug Improvements Summary

## Overview
This document summarizes all changes made to improve LLM context and add comprehensive debugging capabilities.

## 1. Data Structure Changes (pkg/types/fixpatches.go)

### Added Fields to FailedHunk
```go
type FailedHunk struct {
    // ... existing fields ...
    
    // NEW: Expected vs Actual comparison
    ExpectedContext []string // What the patch expects to find
    ActualContext   []string // What's actually in the file
    Differences     []string // Human-readable differences
}
```

**Purpose**: Provide explicit "Expected vs Actual" comparison to help LLM understand exactly why the patch failed.

## 2. Context Extraction Enhancement (pkg/commands/fixpatches/context.go)

### New Function: extractExpectedVsActual()
```go
func extractExpectedVsActual(hunk *types.FailedHunk, actualFileContent string) error
```

**What it does**:
1. Extracts context lines (lines starting with space) from the .rej file
2. These are the lines the patch EXPECTED to find
3. Searches for similar content in the ACTUAL file
4. Identifies specific differences (line numbers, whitespace, content changes)
5. Populates ExpectedContext, ActualContext, and Differences fields

**Example output**:
```
Expected: "github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3..."
Actual:   "github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE="
Difference: "Hash truncated in expected vs actual"
```

## 3. Prompt Enhancement (pkg/commands/fixpatches/llm.go)

### BuildPrompt() - New Section
Added "Expected vs Actual File State" section to the prompt:

```markdown
### Expected vs Actual File State:

**What the patch expects to find:**
```
[context lines from .rej file]
```

**What's actually in the file:**
```
[actual file content]
```

**Key differences:**
- Line numbers shifted by 5 lines
- Extra blank line at line 10
- Hash value truncated
```

**Purpose**: Give LLM explicit visibility into:
- What the patch was looking for
- What's actually there
- Specific differences to address

### Additional Prompt Improvements
1. **Explicit file identification**: "Modify ONLY the file: go.mod"
2. **No assumptions**: "Match the ACTUAL current file state (no blank line assumptions)"
3. **Preserve formatting**: "Preserve the exact formatting and whitespace of the current file"
4. **Complete hashes**: Implicitly handled by showing full actual content

## 4. Debug Logging Enhancements

### A. Prompt and Response Logging (llm.go)
```go
// Write prompt to /tmp/llm-prompt-attempt-N.txt
promptDebugFile := fmt.Sprintf("/tmp/llm-prompt-attempt-%d.txt", attempt)
os.WriteFile(promptDebugFile, []byte(prompt), 0644)

// Write response to /tmp/llm-response-attempt-N.txt
responseDebugFile := fmt.Sprintf("/tmp/llm-response-attempt-%d.txt", attempt)
os.WriteFile(responseDebugFile, []byte(responseText), 0644)
```

**Files created**:
- `/tmp/llm-prompt-attempt-1.txt` - Full prompt sent to LLM
- `/tmp/llm-response-attempt-1.txt` - Full response from LLM
- `/tmp/llm-prompt-attempt-2.txt` - Second attempt (if needed)
- etc.

### B. Patch Debug File (applier.go)
```go
// Save to project directory for easy access
debugPatchFile := filepath.Join(projectPath, ".llm-patch-debug.txt")
os.WriteFile(debugPatchFile, []byte(fix.Patch), 0644)
```

**File created**:
- `projects/<org>/<repo>/.llm-patch-debug.txt` - Last generated patch

### C. Enhanced Console Logging (fixpatches.go)
```go
logger.Info("LLM generated patch fix", 
    "tokens_used", fix.TokensUsed, 
    "cost", fix.Cost,
    "patch_length", len(fix.Patch))  // NEW

logger.Info("Generated patch preview", 
    "preview", patchPreview)  // NEW - first 500 chars

logger.Info("Writing fixed patch to file", 
    "file", patchFile, 
    "patch_length", len(fix.Patch))  // NEW
```

### D. Model Logging (llm.go)
```go
logger.Info("Initialized Bedrock client", 
    "model", model, 
    "profile", modelOrProfile,  // Shows inference profile used
    "region", "us-west-2")
```

## 5. Rate Limiting Improvements

### Global Rate Limiter
```go
var lastRequestTime time.Time
var requestMutex sync.Mutex

func waitForRateLimit() {
    // Ensures 15 seconds between requests (4 requests/min limit)
    minTimeBetweenRequests := 15 * time.Second
    // ... wait logic ...
}
```

**Called before every Bedrock API call** to prevent 503 errors.

## 6. Model Configuration

### Default Model (cmd/fixpatches.go)
```go
fixPatchesCmd.Flags().StringVar(&fixPatchesOptions.Model, 
    "model", 
    "anthropic.claude-sonnet-4-5-20250929-v1:0",  // Claude Sonnet 4.5
    "Bedrock model ID to use")
```

### Inference Profile Mapping (llm.go)
```go
inferenceProfileMap := map[string]string{
    "anthropic.claude-sonnet-4-5-20250929-v1:0": "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
    "anthropic.claude-3-7-sonnet-20250219-v1:0": "us.anthropic.claude-3-7-sonnet-20250219-v1:0",
}
```

## 7. Testing the Changes

### Rebuild Binary
```bash
cd tools/version-tracker
make build
```

### Run with Debug Output
```bash
./bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 1234 \
    --max-attempts 3
```

### Check Debug Files
```bash
# View prompt sent to LLM
cat /tmp/llm-prompt-attempt-1.txt

# View LLM response
cat /tmp/llm-response-attempt-1.txt

# View generated patch
cat test/eks-anywhere-build-tooling/projects/fluxcd/source-controller/.llm-patch-debug.txt

# Check logs
tail -f auto-patch-source-controller.log
```

## 8. What to Look For in Debug Files

### In Prompt File (/tmp/llm-prompt-attempt-1.txt)
✅ Check for "Expected vs Actual File State" section
✅ Verify ExpectedContext shows lines from .rej file
✅ Verify ActualContext shows current file content
✅ Check Differences list is populated
✅ Ensure full hashes are shown (not truncated)

### In Response File (/tmp/llm-response-attempt-1.txt)
✅ Check if LLM generated a complete patch
✅ Verify patch has proper headers (From, Date, Subject)
✅ Check if hashes are complete (not truncated)
✅ Look for any LLM explanations or errors

### In Patch Debug File (.llm-patch-debug.txt)
✅ Verify patch format is correct
✅ Check line numbers match current file
✅ Ensure hashes are complete
✅ Verify all files from original patch are included

## 9. Known Issues Being Addressed

### Issue 1: Model 3.7 Still Logged
**Cause**: Binary not rebuilt after changing default model
**Fix**: Run `make build` in tools/version-tracker

### Issue 2: Patches Not Written
**Cause**: Need to verify WritePatchToFile is being called
**Fix**: Added logging before/after write operation

### Issue 3: Low Context Token Size
**Cause**: Need to verify full prompt is being sent
**Fix**: Write prompt to /tmp file for inspection

### Issue 4: Hash Truncation in go.sum
**Cause**: LLM may be truncating long lines
**Fix**: 
- Show full actual file content in prompt
- Add explicit instruction to preserve exact formatting
- Verify in debug files

### Issue 5: 503 "Too Many Connections"
**Cause**: Rate limiting not working correctly
**Fix**: 
- Added global rate limiter with mutex
- Wait 15 seconds between requests
- Exponential backoff on retries

## 10. Next Steps

1. **Rebuild binary**: `cd tools/version-tracker && make build`
2. **Run test**: Use the test script or manual command
3. **Check debug files**: Inspect /tmp/llm-prompt-attempt-*.txt
4. **Verify context**: Ensure Expected vs Actual section is populated
5. **Check response**: Verify LLM generated complete patch with full hashes
6. **Review logs**: Look for model initialization and rate limiting messages

## Summary of Files Changed

1. `pkg/types/fixpatches.go` - Added ExpectedContext, ActualContext, Differences
2. `pkg/commands/fixpatches/context.go` - Added extractExpectedVsActual()
3. `pkg/commands/fixpatches/llm.go` - Enhanced prompt, added debug logging
4. `pkg/commands/fixpatches/fixpatches.go` - Added patch preview logging
5. `pkg/commands/fixpatches/applier.go` - Added debug patch file
6. `cmd/fixpatches.go` - Updated default model to Sonnet 4.5

All changes are backward compatible and add observability without breaking existing functionality.
