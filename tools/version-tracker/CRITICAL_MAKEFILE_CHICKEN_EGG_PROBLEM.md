# CRITICAL: Makefile Chicken-and-Egg Problem

## Root Cause Found

The "non-existent" GIT_TAG issue is caused by a chicken-and-egg problem in how the fixpatches code interacts with the build system's Makefile.

## The Problem

### What Happens:
1. Fixpatches runs: `make var-value-GIT_TAG` (without RELEASE_BRANCH)
2. Common.mk line 114 filters out `var-value-*` from MAKECMDGOALS
3. Common.mk line 129-140 checks if project has release branches AND no RELEASE_BRANCH is set
4. Since no valid target is detected, it sets `GIT_TAG=non-existent` (line 139)
5. `make var-value-GIT_TAG` returns "non-existent"

### The Chicken-and-Egg:
- To get GIT_TAG, we need to pass RELEASE_BRANCH
- To know if we need RELEASE_BRANCH, we call `make var-value-HAS_RELEASE_BRANCHES`
- But `var-value-HAS_RELEASE_BRANCHES` also gets filtered out and has the same problem!

## The Makefile Logic

```makefile
# Line 114: Filter out var-value-* targets
MAKECMDGOALS_WITHOUT_VAR_VALUE=$(foreach t,$(MAKECMDGOALS),$(if $(findstring var-value-,$(t)),,$(t)))

# Line 129-140: If project has release branches but RELEASE_BRANCH not set
else ifneq ($(IS_RELEASE_BRANCH_BUILD),)
	# avoid warnings when trying to read GIT_TAG file which wont exist when no release_branch is given
	GIT_TAG=non-existent
	OUTPUT_DIR=non-existent
```

## Why This Happens

The Makefile is designed to force users to specify RELEASE_BRANCH for release-branched projects. The `var-value-*` targets are filtered out because they're considered "query" targets that shouldn't trigger the RELEASE_BRANCH requirement.

However, the filtering happens AFTER the conditional check, so the Makefile still thinks you're running a target that requires RELEASE_BRANCH, and sets GIT_TAG to "non-existent" as a safety measure.

## The Solution

### Option 1: Always Pass RELEASE_BRANCH (Recommended)
For projects with release branches, always pass RELEASE_BRANCH when calling ANY make target:

```go
// Get HAS_RELEASE_BRANCHES first (this might also return non-existent!)
hasReleaseBranchesCmd := exec.Command("make", "-C", projectPath, "var-value-HAS_RELEASE_BRANCHES")
// Add a dummy RELEASE_BRANCH to avoid the non-existent trap
hasReleaseBranchesCmd.Env = append(os.Environ(), "RELEASE_BRANCH=1-34")

// Then get GIT_TAG with RELEASE_BRANCH
gitTagCmd := exec.Command("make", "-C", projectPath, "var-value-GIT_TAG")
if hasReleaseBranches {
    gitTagCmd.Env = append(os.Environ(), fmt.Sprintf("RELEASE_BRANCH=%s", releaseBranch))
}
```

### Option 2: Read Files Directly
Skip the Makefile entirely for reading GIT_TAG:

```go
// Read GIT_TAG directly from file
gitTagBytes, err := os.ReadFile(filepath.Join(projectPath, "GIT_TAG"))
if err != nil {
    return nil, nil, fmt.Errorf("reading GIT_TAG: %v", err)
}
gitTag := strings.TrimSpace(string(gitTagBytes))
```

### Option 3: Add var-value-* to TARGETS_ALLOWED_WITH_NO_RELEASE_BRANCH
Modify Common.mk to allow var-value-* targets without RELEASE_BRANCH:

```makefile
TARGETS_ALLOWED_WITH_NO_RELEASE_BRANCH+=var-value-%
```

But this requires changing the build system, which affects all projects.

## Recommended Fix

**Use Option 2 (Read Files Directly)** because:
1. It's the simplest and most reliable
2. Doesn't depend on Makefile quirks
3. Works for all projects consistently
4. GIT_TAG is always a simple file, no complex logic needed

For HAS_RELEASE_BRANCHES, we can still use the Makefile but pass a dummy RELEASE_BRANCH to avoid the trap.

## Implementation

```go
func applySinglePatchWithReject(patchFile string, projectPath string, repoName string) ([]string, *types.PatchApplicationResult, error) {
	logger.Info("Applying single patch with reject", "patch", filepath.Base(patchFile))

	// Read GIT_TAG directly from file (avoid Makefile chicken-and-egg)
	gitTagBytes, err := os.ReadFile(filepath.Join(projectPath, "GIT_TAG"))
	if err != nil {
		return nil, nil, fmt.Errorf("reading GIT_TAG file: %v", err)
	}
	gitTag := strings.TrimSpace(string(gitTagBytes))
	
	// Check if project requires RELEASE_BRANCH
	// Pass a dummy RELEASE_BRANCH to avoid the non-existent trap
	hasReleaseBranchesCmd := exec.Command("make", "-C", projectPath, "var-value-HAS_RELEASE_BRANCHES")
	hasReleaseBranchesCmd.Env = append(os.Environ(), "RELEASE_BRANCH=dummy")
	hasReleaseBranchesOutput, _ := hasReleaseBranchesCmd.CombinedOutput()
	hasReleaseBranches := strings.TrimSpace(string(hasReleaseBranchesOutput)) == "true"

	var releaseBranch string
	if hasReleaseBranches {
		// Get the latest supported release branch
		supportedBranchesFile := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(projectPath))), "release", "SUPPORTED_RELEASE_BRANCHES")
		branchesContent, err := os.ReadFile(supportedBranchesFile)
		if err != nil {
			return nil, nil, fmt.Errorf("reading SUPPORTED_RELEASE_BRANCHES: %v", err)
		}
		branches := strings.Split(strings.TrimSpace(string(branchesContent)), "\n")
		if len(branches) > 0 {
			releaseBranch = strings.TrimSpace(branches[len(branches)-1])
			logger.Info("Project requires RELEASE_BRANCH", "branch", releaseBranch)
		}
	}

	// Build the GIT_CHECKOUT_TARGET
	checkoutTarget := fmt.Sprintf("%s/eks-anywhere-checkout-%s", repoName, gitTag)

	// Ensure the repo is checked out
	checkoutCmd := exec.Command("make", "-C", projectPath, checkoutTarget)
	if releaseBranch != "" {
		checkoutCmd.Env = append(os.Environ(), fmt.Sprintf("RELEASE_BRANCH=%s", releaseBranch))
	}
	// ... rest of the code
}
```

This fix will resolve the "non-existent" issue and allow the kind project patches to be fixed correctly.
