# Autoscaler Special Case - Implementation Plan

## Status

**Cannot implement now** because:
1. Test logs have been cleaned up (`/tmp/llm-*` files gone)
2. PR was manually fixed and merged
3. Cannot run new test (PR no longer exists)
4. File structure appears different than expected

## What We've Prepared

Created `tools/version-tracker/pkg/commands/fixpatches/autoscaler.go` with:
- Detection logic for autoscaler project
- Pattern matching for cloud provider removal patch
- Special case fix logic

## Integration Points (When Ready)

The special case should be integrated into the retry loop:

```go
// In the main patch fixing loop (wherever that is now)
func fixPatchWithRetries(ctx *PatchContext, projectPath string, maxAttempts int) error {
    // Try LLM approach first
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        fix, err := CallBedrockForPatchFix(ctx, model, attempt)
        if err == nil {
            return nil // Success!
        }
        
        // Store error for next attempt
        ctx.BuildError = err.Error()
    }
    
    // All LLM attempts failed - try special case
    if fixedPatch, handled, err := tryAutoscalerSpecialCase(ctx, projectPath); handled {
        if err == nil {
            logger.Info("Autoscaler special case succeeded after LLM failed")
            return applyPatch(fixedPatch, projectPath)
        }
        logger.Info("Autoscaler special case also failed", "error", err)
    }
    
    return fmt.Errorf("all attempts failed")
}
```

## The Autoscaler Pattern

From the README, the cloud provider removal patch:

1. **Removes files**: All `builder_*.go` except `builder_all.go`, `builder_clusterapi.go`, `builder_builder.go`

2. **Updates builder_all.go**:
   - Remove all cloud provider imports except `clusterapi`
   - Remove all entries from `AvailableCloudProviders` except `ClusterAPIProviderName`
   - Remove all cases from `buildCloudProvider` except `clusterapi`
   - Change `DefaultCloudProvider` from `GceProviderName` to `ClusterAPIProviderName`

3. **The Challenge**: New providers (like `coreweave`, `utho`) get added upstream after the patch is created, causing conflicts

## The Special Case Logic

```go
func fixCloudProviderRemovalPatch(ctx *PatchContext) (string, error) {
    // 1. Parse the current file to find ALL cloud providers
    currentProviders := extractProvidersFromFile(ctx.CurrentFileState["builder_all.go"])
    
    // 2. Generate a patch that removes ALL except clusterapi
    var patch strings.Builder
    patch.WriteString(ctx.PatchMetadata) // Preserve metadata
    
    // 3. For each provider (except clusterapi), add removal lines
    for _, provider := range currentProviders {
        if provider != "clusterapi" {
            patch.WriteString(fmt.Sprintf(`-       "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/%s"\n`, provider))
        }
    }
    
    // 4. Keep clusterapi
    patch.WriteString(`        "k8s.io/autoscaler/cluster-autoscaler/cloudprovider/clusterapi"\n`)
    
    // Similar logic for AvailableCloudProviders and buildCloudProvider
    
    return patch.String(), nil
}
```

## When to Implement

**Wait for the next autoscaler version update** that requires patch fixing. Then:

1. Run the test with current LLM optimizations
2. If it still fails, implement the special case
3. Test that it works
4. Measure success rate improvement

## Alternative: Just Document It

Since autoscaler updates are infrequent, you could:

1. **Document the manual process** in the README
2. **Add a check** that detects autoscaler and warns the user
3. **Provide the exact commands** to run manually

This avoids special-case code while still helping future maintainers.

## Recommendation

**Don't implement the special case yet.** Instead:

1. Wait for next autoscaler update
2. Test with current LLM optimizations (they're pretty good now)
3. Only add special case if it still fails
4. This avoids premature optimization

The LLM approach might actually work now with:
- Extended output (128K tokens)
- Dynamic max_tokens (41K for autoscaler)
- Context optimization (only failed files)
- Error positioning (critical errors prominent)
- Conditional original patch (only failed files in retry)

These are significant improvements that might be enough.

## Files Created

- `tools/version-tracker/pkg/commands/fixpatches/autoscaler.go` - Special case logic (ready to integrate)
- This document - Implementation plan

## Next Steps

1. **Wait** for next autoscaler version update
2. **Test** with current optimizations
3. **Measure** success rate
4. **Decide** if special case is needed
5. **Implement** only if necessary
