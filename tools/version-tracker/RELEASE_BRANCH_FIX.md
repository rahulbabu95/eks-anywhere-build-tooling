# Fix: RELEASE_BRANCH Support

## Problem

Some projects like `kubernetes-sigs/kind` have `HAS_RELEASE_BRANCHES=true` in their Makefile, which requires a `RELEASE_BRANCH` environment variable to be set when running make targets.

Error:
```
*** When running targets for this project other than `...` a `RELEASE_BRANCH` is required.  Stop.
```

## Solution

Detect if a project requires `RELEASE_BRANCH` and automatically set it to the latest supported branch.

### Implementation

1. **Check if project has release branches**:
   ```go
   hasReleaseBranchesCmd := exec.Command("make", "-C", projectPath, "var-value-HAS_RELEASE_BRANCHES")
   hasReleaseBranches := strings.TrimSpace(string(output)) == "true"
   ```

2. **Get latest supported branch**:
   ```go
   supportedBranchesFile := "release/SUPPORTED_RELEASE_BRANCHES"
   branches := readFile(supportedBranchesFile)
   releaseBranch := branches[len(branches)-1]  // Use latest
   ```

3. **Set environment variable for make commands**:
   ```go
   checkoutCmd.Env = append(os.Environ(), fmt.Sprintf("RELEASE_BRANCH=%s", releaseBranch))
   ```

## Supported Release Branches

From `release/SUPPORTED_RELEASE_BRANCHES`:
```
1-28
1-29
1-30
1-31
1-32
1-33
1-34
```

We use the **latest** branch (1-34) by default.

## Testing

```bash
cd test/eks-anywhere-build-tooling
SKIP_VALIDATION=true ../../bin/version-tracker fix-patches \
    --project kubernetes-sigs/kind \
    --pr 4789 \
    --max-attempts 3 \
    --verbosity 6
```

Should now work without the RELEASE_BRANCH error.

## Files Modified

- `pkg/commands/fixpatches/fixpatches.go`: Added RELEASE_BRANCH detection and setting

## Notes

- This fix applies to any project with `HAS_RELEASE_BRANCHES=true`
- We automatically use the latest supported release branch
- The RELEASE_BRANCH is set for all make commands in the checkout process
