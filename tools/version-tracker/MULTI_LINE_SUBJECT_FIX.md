# Multi-Line Subject Fix

## Problem Found

The LLM was not preserving the complete commit message because we were only capturing the first line of the Subject header.

### Original Patch:
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
 with correct pseudoversions
```

### What We Captured:
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
```

### What LLM Generated:
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
```

The second line " with correct pseudoversions" was lost!

## Root Cause

In `extractPatchMetadata()`, we were only capturing the first line:

```go
else if match := subjectPattern.FindStringSubmatch(line); match != nil {
    ctx.PatchSubject = match[1]  // Only captures first line!
    // ...
}
```

Git patch format allows Subject headers to span multiple lines. Continuation lines start with a space.

## The Fix

Updated `extractPatchMetadata()` to handle multi-line Subject headers:

```go
var inSubject bool
for scanner.Scan() {
    line := scanner.Text()

    // Stop at the first diff marker
    if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "diff --git") {
        break
    }

    if match := fromPattern.FindStringSubmatch(line); match != nil {
        ctx.PatchAuthor = match[1]
        inSubject = false
    } else if match := datePattern.FindStringSubmatch(line); match != nil {
        ctx.PatchDate = match[1]
        inSubject = false
    } else if match := subjectPattern.FindStringSubmatch(line); match != nil {
        ctx.PatchSubject = match[1]
        inSubject = true  // Mark that we're in Subject
    } else if inSubject && strings.HasPrefix(line, " ") {
        // Continuation of Subject line (starts with space)
        ctx.PatchSubject += "\n" + line
    } else if inSubject && line == "" {
        // Empty line marks end of Subject
        inSubject = false
    }
}

// Extract intent from complete subject (remove [PATCH] prefix if present)
if ctx.PatchSubject != "" {
    subject := strings.TrimSpace(ctx.PatchSubject)
    subject = strings.TrimPrefix(subject, "[PATCH]")
    subject = strings.TrimSpace(subject)
    ctx.PatchIntent = subject
}
```

## How It Works

1. When we match "Subject:", set `inSubject = true`
2. On subsequent lines:
   - If line starts with space → it's a continuation, append it
   - If line is empty → Subject is complete, set `inSubject = false`
   - If line starts with another header → Subject is complete
3. After scanning, extract the intent from the complete Subject

## Expected Result

**Now we capture:**
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
 with correct pseudoversions
```

**LLM will see in prompt:**
```markdown
## Original Patch Metadata
From: Abhay Krishna Arunachalam <arnchlm@amazon.com>
Date: Wed, 7 Feb 2024 22:30:29 -0800
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
 with correct pseudoversions
```

**LLM will preserve:**
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
 with correct pseudoversions
```

## Files Modified

- `tools/version-tracker/pkg/commands/fixpatches/context.go` - Fixed multi-line Subject parsing

## Build Status

✅ Build succeeded
✅ No diagnostics

## Testing

To verify the fix works:

1. Run fix-patches on source-controller
2. Check the generated patch file
3. Verify Subject line includes " with correct pseudoversions"

```bash
cd test/eks-anywhere-build-tooling
../../bin/version-tracker fix-patches --project fluxcd/source-controller --pr 4883

# Check the result
head -10 projects/fluxcd/source-controller/patches/0001-Replace-timestamp-authority-and-go-fuzz-headers-revi.patch
```

Should show:
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
 with correct pseudoversions
```

Not:
```
Subject: [PATCH] Replace timestamp-authority and go-fuzz-headers revisions
```
