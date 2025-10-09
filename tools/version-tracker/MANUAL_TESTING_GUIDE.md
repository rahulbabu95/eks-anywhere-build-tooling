# Manual Testing Guide for LLM Patch Fixer

This guide helps you test the patch-fixer functionality locally using your personal AWS account, without impacting CI/CD or pushing changes to GitHub.

## Key Points

✅ **All changes are LOCAL** - No automatic PR creation or GitHub pushes
✅ **Use your personal AWS account** - Test with Bedrock without affecting production
✅ **Real project data** - Test candidates based on actual patch complexity analysis
✅ **Fast feedback** - Skip validation for quick LLM-only testing

## Quick Start

```bash
# 1. Find PRs with actual patch failures (only these are eligible for testing)
cd tools/version-tracker
./analyze-open-prs.sh

# This will show you PRs like:
# PR #4883 - fluxcd/source-controller (SIMPLE) - 0/1 patches failed
# PR #4789 - kubernetes-sigs/kind (MEDIUM) - 0/6 patches failed

# 2. Pick a SIMPLE project and run a quick test
git checkout -b test-patch-fixer
git fetch origin pull/4883/head:test-pr-4883
git checkout test-pr-4883

SKIP_VALIDATION=true ./version-tracker fix-patches \
  --project fluxcd/source-controller \
  --pr 4883 \
  --max-attempts 1 \
  --verbosity 6

# 3. Review changes (all local, nothing pushed to GitHub)
git diff projects/fluxcd/source-controller/patches/

# 4. Clean up
git reset --hard HEAD
git checkout main
```

## Prerequisites

1. **AWS Account**: Personal AWS account with Bedrock access
2. **AWS Credentials**: Configured via `~/.aws/credentials` or environment variables
3. **Bedrock Model Access**: Request access to Claude Sonnet 4.5 in your AWS account
4. **GitHub Token**: Personal access token with repo permissions
5. **Go**: Go 1.24+ installed

## Setup

### 1. Build the Tool

```bash
cd tools/version-tracker
go build -o version-tracker .
```

### 2. Configure AWS Credentials

```bash
# Option A: Use AWS profile
export AWS_PROFILE=personal

# Option B: Use environment variables
export AWS_ACCESS_KEY_ID=your_key
export AWS_SECRET_ACCESS_KEY=your_secret
export AWS_REGION=us-west-2
```

### 3. Verify Bedrock Access

```bash
aws bedrock list-foundation-models --region us-west-2 | grep claude-sonnet-4
```

If you don't see the model, request access in AWS Console:
- Go to Bedrock → Model access
- Request access to "Claude Sonnet 4.5"

### 4. Set GitHub Token (Optional for now)

```bash
export GITHUB_TOKEN=ghp_your_token_here
```

## Important: Dry-Run Behavior

**All changes are LOCAL only** - the tool does NOT push to GitHub or create PRs automatically. Here's what happens:

1. Tool modifies patch files in your local `projects/<org>/<repo>/patches/` directory
2. Changes remain uncommitted in your local git working directory
3. You can review with `git diff` before deciding to commit/push
4. To test without side effects, work on a separate branch or use `git stash` after testing

**No upstream impact** - You're safe to test freely with your personal AWS account.

## Testing Workflow

### Quick Test (No Validation)

Test the LLM functionality without running builds:

```bash
# 1. Clone the repo (or use your existing clone)
cd /path/to/eks-anywhere-build-tooling

# 2. Create a test branch to avoid impacting your work
git checkout -b test-patch-fixer

# 3. Checkout a PR with failed patches (example: PR #4874)
git fetch origin pull/4874/head:pr-4874
git checkout pr-4874

# 4. Run fix-patches with validation skipped
SKIP_VALIDATION=true ./tools/version-tracker/version-tracker fix-patches \
  --project kubernetes-sigs/cluster-api-provider-vsphere \
  --pr 4874 \
  --max-attempts 3 \
  --verbosity 6

# 5. Check if patches were fixed (all changes are local)
git diff projects/kubernetes-sigs/cluster-api-provider-vsphere/patches/

# 6. Clean up after testing
git reset --hard HEAD
git checkout main
```

### Full Test (With Validation)

Test the complete workflow including builds:

```bash
# Same setup as above, but without SKIP_VALIDATION

./tools/version-tracker/version-tracker fix-patches \
  --project kubernetes-sigs/cluster-api-provider-vsphere \
  --pr 4874 \
  --max-attempts 3 \
  --verbosity 6
```

**Note**: This requires Docker and build dependencies. May take 5-10 minutes per patch.

## Test Candidates

Based on actual project analysis (as of Jan 2025), here are real test candidates:

### Simple (1 patch, <100 lines - good for initial testing)

| Project | Patches | Total Lines | Complexity | Notes |
|---------|---------|-------------|------------|-------|
| fluxcd/source-controller | 1 | 44 | SIMPLE | Good first test |
| kubernetes-sigs/cluster-api-provider-vsphere | 1 | 30 | SIMPLE | Very small, quick test |
| kube-vip/kube-vip | 1 | 49 | SIMPLE | Single small patch |
| replicatedhq/troubleshoot | 1 | 61 | SIMPLE | Good baseline |
| prometheus/prometheus | 1 | 102 | SIMPLE | Slightly larger but manageable |

**Recommended first test**: Use any current open PR for these projects (check https://github.com/aws/eks-anywhere-build-tooling/pulls)

### Medium (2-6 patches, 200-2000 lines)

| Project | Patches | Total Lines | Complexity | Notes |
|---------|---------|-------------|------------|-------|
| emissary-ingress/emissary | 2 | 7,090 | MEDIUM | Tests multi-patch with larger changes |
| kubernetes-sigs/cluster-api-provider-cloudstack | 2 | 2,038 | MEDIUM | Good multi-patch test |
| tinkerbell/ipxedust | 3 | 370 | MEDIUM | Multiple small patches |
| tinkerbell/tink | 3 | 219 | MEDIUM | Multiple patches, moderate size |
| distribution/distribution | 4 | 1,400 | MEDIUM | Tests 4-patch scenario |
| kubernetes-sigs/kind | 6 | 517 | MEDIUM | Multiple patches, reasonable size |

### Complex (Many patches or huge line counts - stress testing only)

| Project | Patches | Total Lines | Complexity | Notes |
|---------|---------|-------------|------------|-------|
| kubernetes-sigs/image-builder | 15 | 1,690 | HIGH | Many patches |
| kubernetes-sigs/cluster-api | 43 | 35,244 | VERY HIGH | 43 patches, 35K lines - extreme case |
| goharbor/harbor | 6 | 209,792 | EXTREME | 6 patches but 210K lines! |

**Warning**: Complex projects may exceed Lambda timeout (20 min) or token limits. Test these last after validating on simpler cases.

### Finding Current Open PRs

Use the helper script to analyze current open PRs and get test recommendations:

```bash
cd tools/version-tracker
./analyze-open-prs.sh
```

This will:
1. Fetch current open PRs from GitHub
2. Match them with local project complexity data
3. Suggest specific test commands for simple and medium complexity projects

Or manually check:
```bash
# List open PRs
curl -s "https://api.github.com/repos/aws/eks-anywhere-build-tooling/pulls?state=open&per_page=50" | \
  jq -r '.[] | "\(.number) | \(.title)"'

# Example recent PRs (as of Jan 2025):
# 4874 | Bump kubernetes-sigs/cluster-api-provider-vsphere to latest release
# 4885 | Bump cert-manager/cert-manager to latest release
# 4891 | Bump fluxcd/kustomize-controller to latest release
```

## Automated Test Script

The script `tools/version-tracker/test-patch-fixer.sh` is already created. To use it:

```bash
cd tools/version-tracker
./test-patch-fixer.sh
```

**Note**: Update the test cases in the script with current open PRs. Example configuration:

```bash
# Test cases: PR_NUMBER:PROJECT_NAME:EXPECTED_COMPLEXITY
TEST_CASES=(
  "4874:kubernetes-sigs/cluster-api-provider-vsphere:simple"
  "4891:fluxcd/kustomize-controller:simple"
  "4885:cert-manager/cert-manager:medium"
)

echo "=== LLM Patch Fixer Manual Testing ==="
echo "Results will be saved to: $TEST_RESULTS_DIR"
echo ""

# Check AWS credentials
if ! aws sts get-caller-identity &>/dev/null; then
  echo "ERROR: AWS credentials not configured"
  exit 1
fi

echo "✓ AWS credentials configured"
echo ""

# Build the tool
echo "Building version-tracker..."
cd tools/version-tracker
go build -o version-tracker .
cd "$REPO_DIR"
echo "✓ Build complete"
echo ""

# Run tests
for test_case in "${TEST_CASES[@]}"; do
  IFS=':' read -r pr_num project expected_complexity <<< "$test_case"
  
  echo "=========================================="
  echo "Testing PR #$pr_num: $project"
  echo "Expected complexity: $expected_complexity"
  echo "=========================================="
  
  # Fetch PR branch
  echo "Fetching PR branch..."
  git fetch origin pull/$pr_num/head:test-pr-$pr_num 2>&1 | head -5
  git checkout test-pr-$pr_num
  
  # Run fix-patches
  echo "Running fix-patches..."
  start_time=$(date +%s)
  
  SKIP_VALIDATION=true ./tools/version-tracker/version-tracker fix-patches \
    --project "$project" \
    --pr "$pr_num" \
    --max-attempts 3 \
    --verbosity 6 \
    2>&1 | tee "$TEST_RESULTS_DIR/pr-$pr_num.log"
  
  exit_code=$?
  end_time=$(date +%s)
  duration=$((end_time - start_time))
  
  # Collect results
  echo ""
  echo "Results for PR #$pr_num:"
  echo "  Exit code: $exit_code"
  echo "  Duration: ${duration}s"
  echo "  Log: $TEST_RESULTS_DIR/pr-$pr_num.log"
  
  # Check if patches were modified
  if git diff --quiet; then
    echo "  Status: ❌ No changes made"
  else
    echo "  Status: ✅ Patches modified"
    git diff --stat > "$TEST_RESULTS_DIR/pr-$pr_num-diff.txt"
    echo "  Diff: $TEST_RESULTS_DIR/pr-$pr_num-diff.txt"
  fi
  
  echo ""
  
  # Reset for next test
  git reset --hard HEAD
  git checkout main
  
  # Pause between tests
  sleep 2
done

echo "=========================================="
echo "Testing complete!"
echo "Results saved to: $TEST_RESULTS_DIR"
echo ""
echo "Summary:"
ls -lh "$TEST_RESULTS_DIR"
```

Make it executable:
```bash
chmod +x tools/version-tracker/test-patch-fixer.sh
```

## Manual Testing Steps

### Step 1: Quick Smoke Test (5 minutes)

```bash
# Test on simplest PR (use a current open PR)
cd /path/to/eks-anywhere-build-tooling

# Create test branch
git checkout -b test-patch-fixer

# Fetch a simple PR (example: PR #4874 for cluster-api-provider-vsphere)
git fetch origin pull/4874/head:test-pr-4874
git checkout test-pr-4874

# Run with validation skipped for fast feedback
SKIP_VALIDATION=true ./tools/version-tracker/version-tracker fix-patches \
  --project kubernetes-sigs/cluster-api-provider-vsphere \
  --pr 4874 \
  --max-attempts 1 \
  --verbosity 6

# Check what changed (all local, not pushed)
git diff projects/kubernetes-sigs/cluster-api-provider-vsphere/patches/

# Clean up
git reset --hard HEAD
```

**Expected output:**
- ✅ Finds patch files (1 patch, ~30 lines for this project)
- ✅ Applies patch with --reject to generate .rej files
- ✅ Calculates complexity score
- ✅ Calls Bedrock API
- ✅ Applies LLM-generated fix to patch file
- ✅ Logs cost and tokens used

### Step 2: Run Automated Tests (30 minutes)

First, update test cases in the script with current open PRs, then run:

```bash
cd tools/version-tracker
./test-patch-fixer.sh
```

Review results in `patch-fixer-test-results/`

### Step 3: Full Validation Test (1 hour)

Pick one successful PR from Step 2 and run with full validation:

```bash
git checkout test-pr-4874

# Run with validation (requires Docker and build dependencies)
./tools/version-tracker/version-tracker fix-patches \
  --project kubernetes-sigs/cluster-api-provider-vsphere \
  --pr 4874 \
  --max-attempts 3 \
  --verbosity 6

# This will run 'make build' and 'make checksums' to verify the fix
```

## Metrics to Collect

For each test, record these metrics to help refine the system:

```
PR: #4874
Project: kubernetes-sigs/cluster-api-provider-vsphere
Patches: 1
Total Lines: 30
Complexity Score: 2
Attempts: 1
Success: Yes
Duration: 45s
Cost: $0.85
Tokens: 8,450 input + 650 output
Notes: Single small patch, fixed go.mod conflict
```

Create a spreadsheet or CSV to track patterns:

```csv
PR,Project,Patches,TotalLines,ComplexityScore,Attempts,Success,Duration,Cost,InputTokens,OutputTokens,Notes
4874,kubernetes-sigs/cluster-api-provider-vsphere,1,30,2,1,Yes,45,$0.85,8450,650,Single small patch
4891,fluxcd/kustomize-controller,1,44,2,1,Yes,52,$0.92,9200,720,go.mod update
4885,cert-manager/cert-manager,11,XXX,XX,3,Partial,180,$3.40,35000,2800,Multiple files
```

**Key metrics to analyze:**
- Success rate by complexity score
- Average cost per patch
- Token usage patterns
- Correlation between patch size and success rate

## Troubleshooting

### Issue: "AWS credentials not configured"
```bash
aws configure
# Or set AWS_PROFILE
```

### Issue: "Bedrock model not accessible"
- Go to AWS Console → Bedrock → Model access
- Request access to Claude Sonnet 4.5
- Wait for approval (usually instant)

### Issue: "make build failed"
- Ensure Docker is running
- Check project-specific build requirements
- Use `SKIP_VALIDATION=true` to bypass

### Issue: "No .rej files found"
- The PR might not have patch conflicts
- Check PR comments for "Failed patch details"
- Try a different PR

## Quick Reference

### Environment Variables

```bash
# Skip build validation (for faster testing)
export SKIP_VALIDATION=true

# AWS configuration
export AWS_PROFILE=personal
export AWS_REGION=us-west-2

# GitHub token (optional for now)
export GITHUB_TOKEN=ghp_xxx
```

### Useful Commands

```bash
# Check what patches exist
ls -la projects/kubernetes-sigs/cluster-api/patches/

# See if .rej files were generated
find projects/kubernetes-sigs/cluster-api/cluster-api -name "*.rej"

# View the diff after fix
git diff projects/kubernetes-sigs/cluster-api/cluster-api/

# Reset to clean state
git reset --hard HEAD
git clean -fd
```

## Next Steps After Manual Testing

1. **Analyze metrics**: Success rates, costs, patterns
2. **Refine prompts**: Based on failure cases
3. **Adjust complexity threshold**: Based on success correlation
4. **Implement full validation**: If builds work reliably
5. **Move to Lambda**: Once confident in core functionality

## Support

If you encounter issues:
1. Check logs with `--verbosity 6`
2. Review `patch-fixer-test-results/*.log` files
3. Verify AWS Bedrock permissions
4. Test with simpler PRs first
