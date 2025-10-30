# Session Summary & Next Steps

## What We Accomplished

### 1. Identified Critical Issues Through Analysis

**Issue 1: Prompt Overfitting**
- Original patch section was causing LLM to copy truncated hashes
- Removed it, but then LLM lost context about all files in patch

**Issue 2: BuildError Not Persisting**
- Context was recreated each iteration, losing error information
- Fixed by storing error state outside loop

**Issue 3: Missing File Context**
- Only showing context for files with .rej files
- go.sum applied with offset but had no context shown
- LLM couldn't fix what it couldn't see

### 2. Implemented Several Improvements

**A. Context Enhancement (pkg/types/fixpatches.go)**
- Added `ExpectedContext`, `ActualContext`, `Differences` to `FailedHunk`
- Added `PatchApplicationResult` type to track offset files
- Added `ApplicationResult` field to `PatchContext`

**B. Offset Detection (pkg/commands/fixpatches/fixpatches.go)**
- Parse git apply output to detect offset hunks
- Regex: `Hunk #\d+ succeeded at \d+ \(offset (\d+) lines?\)`
- Track which files applied with offset

**C. Error Persistence (pkg/commands/fixpatches/fixpatches.go)**
- Store `lastBuildError` and `previousAttempts` outside loop
- Restore to context after recreation
- Ensures error information flows between attempts

**D. Prompt Improvements (pkg/commands/fixpatches/llm.go)**
- Removed biased examples
- Added actual error messages to prompts
- Simplified instructions
- Added back original patch with status information

### 3. Discovered Remaining Critical Issue

**The Problem**: Even with improvements, LLM still can't fix patches because:

1. **Stale Status**: Prompt shows "go.mod FAILED" in attempt 2, but go.mod succeeded in attempt 1
2. **Missing go.sum Context**: No current file content shown for go.sum around line 933
3. **Error Not Used**: Git apply error says "go.sum:933" but we don't extract context for it

**Root Cause**: We only extract context for files with .rej files. Files that apply with offset or fail in later attempts have no context.

---

## Decision: Implement Approach 2

### The Strategy

**"Parse original patch to find ALL files, extract context for each file around the lines being changed"**

### Why Approach 2

- ✅ Straightforward to implement
- ✅ Generic - works for any file type
- ✅ Proactive - LLM sees all context from attempt 1
- ✅ Complete - covers all files in patch

### Token Management

To avoid context overload:
- Extract only ±10 lines around changed lines (not entire file)
- Skip files that applied cleanly (no .rej, no offset)
- Limit context per file to ~50 lines max

---

## Implementation Plan for Next Session

### Phase 1: Parse Patch to Find All Files

**File**: `tools/version-tracker/pkg/commands/fixpatches/context.go`

**Add function**:
```go
// PatchFile represents a file being modified in a patch
type PatchFile struct {
    Path       string
    LineRanges []LineRange // Lines being changed
}

type LineRange struct {
    Start int
    End   int
}

// parsePatchFiles extracts all files and their changed line ranges from a patch
func parsePatchFiles(patchContent string) ([]PatchFile, error) {
    files := make([]PatchFile, 0)
    
    // Parse unified diff format
    // Look for: diff --git a/file b/file
    // Then: @@ -oldStart,oldCount +newStart,newCount @@
    
    scanner := bufio.NewScanner(strings.NewReader(patchContent))
    var currentFile *PatchFile
    
    for scanner.Scan() {
        line := scanner.Text()
        
        // New file
        if strings.HasPrefix(line, "diff --git") {
            if currentFile != nil {
                files = append(files, *currentFile)
            }
            // Extract filename from "diff --git a/file b/file"
            parts := strings.Fields(line)
            if len(parts) >= 4 {
                filename := strings.TrimPrefix(parts[3], "b/")
                currentFile = &PatchFile{
                    Path: filename,
                    LineRanges: make([]LineRange, 0),
                }
            }
        }
        
        // Hunk header: @@ -10,5 +12,7 @@
        if strings.HasPrefix(line, "@@") && currentFile != nil {
            // Parse to get new line range (+12,7 means start at 12, 7 lines)
            re := regexp.MustCompile(`\+(\d+),(\d+)`)
            matches := re.FindStringSubmatch(line)
            if len(matches) >= 3 {
                start, _ := strconv.Atoi(matches[1])
                count, _ := strconv.Atoi(matches[2])
                currentFile.LineRanges = append(currentFile.LineRanges, LineRange{
                    Start: start,
                    End:   start + count,
                })
            }
        }
    }
    
    // Don't forget last file
    if currentFile != nil {
        files = append(files, *currentFile)
    }
    
    return files, nil
}
```

### Phase 2: Extract Context for Each File

**File**: `tools/version-tracker/pkg/commands/fixpatches/context.go`

**Add function**:
```go
// extractContextForAllFiles extracts current file content for all files in patch
func extractContextForAllFiles(patchFiles []PatchFile, projectPath string, repoName string) map[string]string {
    contexts := make(map[string]string)
    
    repoPath := filepath.Join(projectPath, repoName)
    
    for _, patchFile := range patchFiles {
        filePath := filepath.Join(repoPath, patchFile.Path)
        
        // Read file
        content, err := os.ReadFile(filePath)
        if err != nil {
            logger.Info("Warning: could not read file", "file", patchFile.Path, "error", err)
            continue
        }
        
        lines := strings.Split(string(content), "\n")
        
        // Extract context around each changed line range
        var contextLines []string
        for _, lineRange := range patchFile.LineRanges {
            // Get ±10 lines around the change
            start := max(0, lineRange.Start-10)
            end := min(len(lines), lineRange.End+10)
            
            contextLines = append(contextLines, fmt.Sprintf("Lines %d-%d:", start+1, end))
            for i := start; i < end; i++ {
                contextLines = append(contextLines, lines[i])
            }
            contextLines = append(contextLines, "") // Blank line between ranges
        }
        
        contexts[patchFile.Path] = strings.Join(contextLines, "\n")
    }
    
    return contexts
}
```

### Phase 3: Update ExtractPatchContext

**File**: `tools/version-tracker/pkg/commands/fixpatches/context.go`

**Modify `ExtractPatchContext` function**:
```go
func ExtractPatchContext(...) (*types.PatchContext, error) {
    // ... existing code ...
    
    // NEW: Parse patch to find all files
    patchFiles, err := parsePatchFiles(ctx.OriginalPatch)
    if err != nil {
        logger.Info("Warning: could not parse patch files", "error", err)
    } else {
        // Extract context for all files
        repoName := filepath.Base(projectPath)
        allFileContexts := extractContextForAllFiles(patchFiles, projectPath, repoName)
        
        // Store in context
        ctx.AllFileContexts = allFileContexts
    }
    
    // ... rest of existing code ...
}
```

### Phase 4: Update PatchContext Type

**File**: `tools/version-tracker/pkg/types/fixpatches.go`

**Add field**:
```go
type PatchContext struct {
    // ... existing fields ...
    AllFileContexts map[string]string // filename -> current content around changed lines
}
```

### Phase 5: Update Prompt to Show All File Contexts

**File**: `tools/version-tracker/pkg/commands/fixpatches/llm.go`

**In `BuildPrompt` function, add section**:
```go
// After showing failed hunks, before original patch

// Show context for ALL files in patch
if len(ctx.AllFileContexts) > 0 {
    prompt.WriteString("## Current File States\n\n")
    prompt.WriteString("Here is the current content of all files being modified:\n\n")
    
    for filename, context := range ctx.AllFileContexts {
        prompt.WriteString(fmt.Sprintf("### %s\n\n", filename))
        
        // Check if this file has a .rej (failed)
        hasFailed := false
        for _, hunk := range ctx.FailedHunks {
            if strings.Contains(hunk.FilePath, filename) {
                hasFailed = true
                break
            }
        }
        
        // Check if this file has offset
        hasOffset := false
        offsetAmount := 0
        if ctx.ApplicationResult != nil {
            if offset, ok := ctx.ApplicationResult.OffsetFiles[filename]; ok {
                hasOffset = true
                offsetAmount = offset
            }
        }
        
        // Show status
        if hasFailed {
            prompt.WriteString("**Status**: ❌ FAILED (see detailed context above)\n\n")
        } else if hasOffset {
            prompt.WriteString(fmt.Sprintf("**Status**: ⚠️ APPLIED WITH OFFSET (+%d lines)\n\n", offsetAmount))
        } else {
            prompt.WriteString("**Status**: ✅ APPLIED CLEANLY\n\n")
        }
        
        // Show current content
        prompt.WriteString("**Current content:**\n")
        prompt.WriteString("```\n")
        prompt.WriteString(context)
        prompt.WriteString("\n```\n\n")
    }
}
```

---

## Testing After Implementation

### 1. Build
```bash
cd tools/version-tracker
make build
```

### 2. Run Test
```bash
cd test/eks-anywhere-build-tooling
./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-$(date +%Y%m%d-%H%M%S).log
```

### 3. Verify Prompt
```bash
cat /tmp/llm-prompt-attempt-1.txt
```

**Should show**:
```markdown
## Failed Hunk #1 in go.mod
[go.mod context with Expected vs Actual]

## Current File States

### go.mod
**Status**: ❌ FAILED
**Current content:**
[lines around the change]

### go.sum
**Status**: ⚠️ APPLIED WITH OFFSET (+2 lines)
**Current content:**
[lines 923-943 showing context around line 933]

## Original Patch (For Reference)
[full patch]
```

---

## Expected Improvements

### Before (Current State)
- ❌ Only go.mod context shown
- ❌ No go.sum context
- ❌ LLM can't fix go.sum

### After (With Approach 2)
- ✅ Both go.mod and go.sum context shown
- ✅ LLM sees actual content at line 933
- ✅ LLM can generate correct patch for both files

---

## Key Files to Modify

1. **pkg/types/fixpatches.go**
   - Add `AllFileContexts map[string]string` to `PatchContext`

2. **pkg/commands/fixpatches/context.go**
   - Add `parsePatchFiles()` function
   - Add `extractContextForAllFiles()` function
   - Update `ExtractPatchContext()` to call these

3. **pkg/commands/fixpatches/llm.go**
   - Update `BuildPrompt()` to show all file contexts

---

## Token Management Strategy

To keep token usage reasonable:

1. **Limit context per file**: ±10 lines around changes (not entire file)
2. **Skip successful files**: If no .rej and no offset, skip context
3. **Truncate large contexts**: Max 50 lines per file
4. **Prioritize failed files**: Show full context for failed, abbreviated for offset

**Estimated token usage**:
- Current: ~2,000 tokens (only go.mod)
- With Approach 2: ~4,000 tokens (go.mod + go.sum)
- Still well under 200K limit

---

## Future Enhancements (Post-Approach 2)

Once Approach 2 is working:

1. **Smart context extraction**: Only extract for files that need it (hybrid approach)
2. **Diff-based context**: Show what changed between expected and actual
3. **MCP server**: Let LLM request specific file content on demand
4. **Caching**: Cache file content to avoid re-reading

---

## Summary

**Current State**: LLM can't fix patches because it's missing go.sum context

**Next Step**: Implement Approach 2 to parse patch and extract context for ALL files

**Implementation**: 5 phases, modify 3 files, add ~150 lines of code

**Expected Result**: LLM sees complete context and can generate correct patches

**Token Impact**: Minimal (~2x current usage, still well under limits)

---

## Quick Start for Next Session

1. Read this document
2. Start with Phase 1: Add `parsePatchFiles()` to context.go
3. Test parsing with the source-controller patch
4. Continue through phases 2-5
5. Build and test
6. Check /tmp/llm-prompt-attempt-1.txt to verify all file contexts are shown

Good luck! The implementation is straightforward - just parsing unified diff format and reading file content.
