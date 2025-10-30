# Approach Analysis - Generic Context Extraction

## The Core Problem

**We need to provide the LLM with context about ALL files being modified, not just files with .rej files.**

The current approach is reactive (wait for .rej files). We need a proactive approach that works for:
- Go module files (go.mod, go.sum)
- Source code changes (*.go, *.py, *.js)
- API definitions (*.proto, *.yaml)
- Configuration files (*.json, *.toml)
- Documentation (*.md)
- Any file type

## Available Approaches

### Approach 1: Parse Git Apply Errors (Reactive)

**How it works**:
```
1. LLM generates patch
2. Try to apply → fails
3. Parse error: "error: patch failed: file.go:123"
4. Extract context from file.go around line 123
5. Show to LLM in next attempt
```

**Pros**:
- ✅ Simple to implement
- ✅ Only extracts context for files that actually fail
- ✅ Minimal token usage

**Cons**:
- ❌ Reactive - only helps on retry
- ❌ LLM doesn't see context until after it fails
- ❌ Doesn't help with offset files
- ❌ Requires parsing error messages (fragile)

**Generic?**: ⚠️ Partially - works for any file type but only after failure

---

### Approach 2: Parse Original Patch for All Files (Proactive)

**How it works**:
```
1. Parse original patch to find ALL files being modified
2. For each file:
   a. Extract the line ranges being changed
   b. Read current file content around those lines
   c. Include in context
3. Show all file contexts to LLM upfront
```

**Pros**:
- ✅ Proactive - LLM sees everything from attempt 1
- ✅ Works for all file types
- ✅ Handles offset files naturally
- ✅ No error parsing needed

**Cons**:
- ❌ Higher token usage (showing all files)
- ❌ Need to parse patch format correctly
- ❌ May include unnecessary context for files that apply cleanly

**Generic?**: ✅ Yes - works for any file in any patch

---

### Approach 3: Hybrid - Parse Patch + Track Application Results

**How it works**:
```
1. Parse original patch to find all files
2. Apply patch and track results per file:
   - SUCCESS: applied cleanly
   - OFFSET: applied with line offset
   - FAILED: created .rej file
3. Extract context based on status:
   - SUCCESS: no context needed
   - OFFSET: extract context around offset lines
   - FAILED: extract context from .rej + current file
4. Show status + relevant context to LLM
```

**Pros**:
- ✅ Proactive for offset/failed files
- ✅ Efficient - only extracts context where needed
- ✅ Works for all file types
- ✅ Clear status per file

**Cons**:
- ❌ More complex implementation
- ❌ Need to parse both patch and git apply output

**Generic?**: ✅ Yes - works for any file type

---

### Approach 4: Always Extract Context for All Files (Comprehensive)

**How it works**:
```
1. Parse original patch to find all files and line ranges
2. For EVERY file, extract:
   - Lines being added/removed
   - Context around those lines (±10 lines)
   - Current file state
3. Show complete context for all files
4. Let LLM decide what needs fixing
```

**Pros**:
- ✅ Most complete context
- ✅ LLM has full picture
- ✅ Works for any file type
- ✅ No guessing about what's needed

**Cons**:
- ❌ Highest token usage
- ❌ May exceed context limits for large patches
- ❌ Includes unnecessary context for clean files

**Generic?**: ✅ Yes - most generic approach

---

### Approach 5: Incremental Context (Adaptive)

**How it works**:
```
Attempt 1:
- Show only .rej file context (minimal)

Attempt 2:
- Parse error from attempt 1
- Add context for files mentioned in error
- Show accumulated context

Attempt 3:
- Parse error from attempt 2
- Add more context as needed
- Show accumulated context
```

**Pros**:
- ✅ Starts minimal, grows as needed
- ✅ Efficient token usage
- ✅ Adapts to specific failure

**Cons**:
- ❌ Slow - requires multiple attempts
- ❌ Complex state management
- ❌ May never converge

**Generic?**: ⚠️ Partially - depends on error parsing

---

## Recommended Solution: Approach 2 + Approach 3 Hybrid

### The Strategy

**"Parse patch upfront, extract context for all modified files, categorize by application status"**

### Implementation

```go
type FileContext struct {
    FilePath       string
    Status         string // "SUCCESS", "OFFSET", "FAILED"
    OffsetLines    int    // For OFFSET status
    ChangedLines   []int  // Line numbers being modified
    CurrentContent string // Content around changed lines
    ExpectedContent string // What patch expects (from .rej or patch)
    Differences    []string
}

func ExtractCompleteContext(patch string, applyResult *PatchApplicationResult, rejFiles []string) map[string]*FileContext {
    contexts := make(map[string]*FileContext)
    
    // Step 1: Parse patch to find all files
    files := parsePatchFiles(patch)
    
    for _, file := range files {
        ctx := &FileContext{
            FilePath: file.Path,
            ChangedLines: file.LineRanges,
        }
        
        // Step 2: Determine status
        if hasRejFile(file.Path, rejFiles) {
            ctx.Status = "FAILED"
            ctx.ExpectedContent = extractFromRejFile(file.Path, rejFiles)
        } else if offset, hasOffset := applyResult.OffsetFiles[file.Path]; hasOffset {
            ctx.Status = "OFFSET"
            ctx.OffsetLines = offset
        } else {
            ctx.Status = "SUCCESS"
            // Skip context extraction for successful files
            continue
        }
        
        // Step 3: Extract current file content
        ctx.CurrentContent = extractFileContext(file.Path, file.LineRanges)
        
        // Step 4: Compare expected vs actual
        if ctx.Status == "FAILED" {
            ctx.Differences = compareExpectedVsActual(ctx.ExpectedContent, ctx.CurrentContent)
        }
        
        contexts[file.Path] = ctx
    }
    
    return contexts
}
```

### Why This Works

1. **Generic**: Works for any file type - just reads lines
2. **Efficient**: Only extracts context for files that need it
3. **Complete**: Covers all cases (failed, offset, success)
4. **Scalable**: Can handle patches with many files
5. **Proactive**: LLM sees all relevant context from attempt 1

### Prompt Structure

```markdown
## Files Being Modified

### go.mod
**Status**: ❌ FAILED
**Lines being changed**: 8-11

**Expected (from patch):**
```
[context from .rej]
```

**Current file state:**
```
[actual content around line 8-11]
```

**Differences:**
- Line 10: version changed from v0.8.0 to v0.9.0
- Line 11: version changed from v1.56.1 to v1.57.0

---

### go.sum
**Status**: ⚠️ APPLIED WITH OFFSET
**Lines being changed**: 933-935 (offset: +2 lines, now at 935-937)

**Current file state at lines 933-937:**
```
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5 h1:7U0GsO0UGG1PdtgS6wBkRC0sMgq7BRVaFlPRwN4m1Qg=
github.com/sigstore/sigstore/pkg/signature/kms/gcp v1.9.5/go.mod h1:/2qrI0nnCy/DTIPOMFaZlFnNPWEn5UeS70P37XEM88o=
github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5 h1:S2ukEfN1orLKw2wEQIUHDDlzk0YcylhcheeZ5TGk8LI=
github.com/sigstore/sigstore/pkg/signature/kms/hashivault v1.9.5/go.mod h1:m7sQxVJmDa+rsmS1m6biQxaLX83pzNS7ThUEyjOqkCU=
github.com/sigstore/timestamp-authority v1.2.8 h1:BEV3fkphwU4zBp3allFAhCqQb99HkiyCXB853RIwuEE=
```

**What needs to change:**
The patch expects to change lines 933-935, but due to file growth, these are now at lines 935-937.
Update the line numbers in your patch to match the current file.

---

## Original Patch (For Reference)
[full patch]

## Task
Generate a corrected patch that:
1. For FAILED files: Fix using the Expected vs Current comparison
2. For OFFSET files: Update line numbers to match current file state
3. Include ALL files from the original patch
```

---

## Alternative: Simpler Version (Approach 2 Only)

If the hybrid is too complex, we can simplify to just Approach 2:

```go
func ExtractAllFileContexts(patch string, projectPath string) map[string]*FileContext {
    contexts := make(map[string]*FileContext)
    
    // Parse patch to find all files and line ranges
    files := parsePatchFiles(patch)
    
    for _, file := range files {
        // Extract current content around changed lines
        content := extractFileContext(file.Path, file.LineRanges, projectPath)
        
        contexts[file.Path] = &FileContext{
            FilePath: file.Path,
            ChangedLines: file.LineRanges,
            CurrentContent: content,
        }
    }
    
    return contexts
}
```

**Simpler but**:
- Shows context for ALL files (even successful ones)
- Higher token usage
- But guaranteed to have all context

---

## Comparison Matrix

| Approach | Generic | Proactive | Efficient | Simple | Scalable |
|----------|---------|-----------|-----------|--------|----------|
| 1. Parse Errors | ⚠️ | ❌ | ✅ | ✅ | ✅ |
| 2. Parse Patch | ✅ | ✅ | ⚠️ | ✅ | ⚠️ |
| 3. Hybrid | ✅ | ✅ | ✅ | ❌ | ✅ |
| 4. All Files | ✅ | ✅ | ❌ | ✅ | ❌ |
| 5. Incremental | ⚠️ | ❌ | ✅ | ❌ | ⚠️ |

---

## Recommendation

**Implement Approach 3 (Hybrid)** because:

1. **Most generic**: Works for any file type
2. **Most efficient**: Only extracts context where needed
3. **Most complete**: Handles all cases (failed, offset, success)
4. **Scalable**: Can handle large patches without token explosion
5. **Proactive**: LLM sees all relevant context from attempt 1

### Implementation Plan

**Phase 1**: Parse patch to find all files
- Create `parsePatchFiles()` function
- Extract file paths and line ranges from unified diff format

**Phase 2**: Categorize files by status
- Use existing .rej file detection
- Use existing offset detection
- Mark remaining as SUCCESS

**Phase 3**: Extract context per status
- FAILED: Extract from .rej + current file (already doing this)
- OFFSET: Extract current file content around offset lines (NEW)
- SUCCESS: Skip (no context needed)

**Phase 4**: Update prompt to show all file contexts
- Show status for each file
- Show context for FAILED and OFFSET files
- Clear instructions for each type

This gives us a generic, scalable solution that works for:
- ✅ Go modules
- ✅ Source code
- ✅ API definitions
- ✅ Config files
- ✅ Any text file in a patch

---

## Future: MCP Server Approach

When we move to MCP server, we can expose tools:

```typescript
{
  "name": "get_file_content",
  "description": "Get current content of a file around specific lines",
  "parameters": {
    "file": "string",
    "start_line": "number",
    "end_line": "number"
  }
}

{
  "name": "get_patch_status",
  "description": "Get application status for all files in patch",
  "returns": {
    "files": [
      {"path": "go.mod", "status": "FAILED", "line": 8},
      {"path": "go.sum", "status": "OFFSET", "offset": 2}
    ]
  }
}
```

The LLM can then request exactly the context it needs, when it needs it.

But for now, the Hybrid approach gives us the best balance.
