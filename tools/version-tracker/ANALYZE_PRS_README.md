# analyze-open-prs.sh - Find Test Candidates

## Purpose

This script identifies **eligible test candidates** for the LLM patch fixer by:
1. Fetching open PRs from `eks-distro-pr-bot` only
2. Checking for bot comments indicating patch application failures
3. Matching with local project complexity data
4. Providing ready-to-use test commands

## Usage

```bash
cd tools/version-tracker
./analyze-open-prs.sh
```

## Output

The script provides three sections:

### 1. Summary Table

Shows all PRs with patch failures, sorted by complexity:

```
PR#    | Project                                       |  Patches |  Lines | Complex  | Failed
-------|-----------------------------------------------|----------|--------|----------|------------------
4883   | fluxcd/source-controller                      |  1 patch |     44 | SIMPLE   | 0/1
4789   | kubernetes-sigs/kind                          |  6 patch |    517 | MEDIUM   | 0/6
4656   | kubernetes-sigs/cluster-api                   | 43 patch |  35244 | HIGH     | 2/40
```

- **Failed**: Shows "applied/total" patches (e.g., "0/1" means 0 applied, 1 failed)

### 2. Recommended Test Commands

Ready-to-run commands organized by complexity:

```bash
# SIMPLE projects (good for initial testing):
./test-patch-fixer.sh fluxcd/source-controller 4883
  # Failed: 0/1 patches, Files: go.mod

# MEDIUM projects (for comprehensive testing):
./test-patch-fixer.sh kubernetes-sigs/kind 4789
  # Failed: 0/6 patches, Files: images/base/Dockerfile

# HIGH complexity projects (stress testing):
./test-patch-fixer.sh kubernetes-sigs/cluster-api 4656
  # Failed: 2/40 patches, Files: controlplane/kubeadm/internal/controllers/controller.go
```

### 3. Detailed Failure Information

Full details for each PR:

```
PR #4883 - fluxcd/source-controller (SIMPLE)
  Total patches: 1 (44 lines)
  Failed: 0/1
  Failed at: 0001 Replace timestamp-authority and go-fuzz-headers revisions
  Files: go.mod
  URL: https://github.com/aws/eks-anywhere-build-tooling/pull/4883
```

## How It Works

1. **Fetches PRs**: Gets all open PRs from `eks-distro-pr-bot` (typically 40-50 PRs)
2. **Checks for failures**: Looks for bot comments containing "Failed patch details"
3. **Extracts details**: Parses failure information:
   - Number of patches applied/failed
   - Which patch failed
   - Which files had conflicts
4. **Calculates complexity**: Based on local patch files:
   - SIMPLE: <100 lines
   - MEDIUM: 100-2000 lines
   - HIGH: >2000 lines
5. **Generates commands**: Provides ready-to-use test commands

## Example Real Output (Jan 2025)

Current eligible test candidates:

**SIMPLE (2 PRs)**:
- PR #4883: fluxcd/source-controller (1 patch, 44 lines, 0/1 failed)
- PR #4408: aquasecurity/trivy (1 patch, 41 lines, 0/1 failed)

**MEDIUM (7 PRs)**:
- PR #4861: nutanix-cloud-native/cluster-api-provider-nutanix (1 patch, 118 lines, 0/1 failed)
- PR #4789: kubernetes-sigs/kind (6 patches, 517 lines, 0/6 failed)
- PR #4757: kubernetes-sigs/image-builder (15 patches, 1690 lines, 10/13 failed)

**HIGH (3 PRs)**:
- PR #4656: kubernetes-sigs/cluster-api (43 patches, 35K lines, 2/40 failed)
- PR #4512: goharbor/harbor (6 patches, 210K lines, 2/3 failed)
- PR #4501: emissary-ingress/emissary (2 patches, 7K lines, 0/1 failed)

## Why This Matters

**Before this script**: You might test PRs that don't have patch failures, wasting time and AWS credits.

**With this script**: You only test PRs that actually need the LLM patch fixer, ensuring:
- Efficient use of testing time
- Accurate success rate metrics
- Relevant test data for prompt optimization

## Requirements

- `curl` - for GitHub API calls
- `jq` - for JSON parsing
- Internet connection - to fetch PR data
- Local project directories - to calculate complexity

## Troubleshooting

**"No PRs with patch failures found"**
- All current PRs may have patches applying successfully
- Bot may not have commented yet on recent PRs
- Check manually: https://github.com/aws/eks-anywhere-build-tooling/pulls

**"Failed to fetch PRs from GitHub"**
- Check internet connection
- GitHub API may be rate-limited (60 requests/hour without auth)
- Try again in a few minutes

**Slow execution**
- Script makes API calls for each PR to check comments
- Typical runtime: 30-60 seconds for 40 PRs
- This is normal and expected
