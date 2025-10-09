#!/bin/bash
set -e

# Configuration
REPO_DIR=$(git rev-parse --show-toplevel)
TEST_RESULTS_DIR="$REPO_DIR/patch-fixer-test-results"
mkdir -p "$TEST_RESULTS_DIR"

# Test cases: PR_NUMBER:PROJECT_NAME:EXPECTED_COMPLEXITY
# Update these with current open PRs from: https://github.com/aws/eks-anywhere-build-tooling/pulls
# 
# Project complexity reference (based on actual patch analysis):
# SIMPLE: 1 patch, <100 lines (e.g., cluster-api-provider-vsphere: 1 patch, 30 lines)
# MEDIUM: 2-6 patches, 200-2000 lines (e.g., emissary: 2 patches, 7090 lines)
# HIGH: 15+ patches or 2000+ lines (e.g., cluster-api: 43 patches, 35244 lines)
#
# Current open PRs (as of Jan 2025):
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
  echo "Run: aws configure"
  echo "Or set: export AWS_PROFILE=personal"
  exit 1
fi

echo "✓ AWS credentials configured"
aws sts get-caller-identity | grep UserId
echo ""

# Check Bedrock access
echo "Checking Bedrock model access..."
if aws bedrock list-foundation-models --region us-west-2 2>/dev/null | grep -q "claude-sonnet"; then
  echo "✓ Bedrock access confirmed"
else
  echo "⚠ Warning: Could not verify Bedrock access"
  echo "  Make sure you have access to Claude models in Bedrock"
fi
echo ""

# Build the tool
echo "Building version-tracker..."
cd "$REPO_DIR/tools/version-tracker"
go build -o version-tracker .
TOOL_PATH="$REPO_DIR/tools/version-tracker/version-tracker"
cd "$REPO_DIR"
echo "✓ Build complete: $TOOL_PATH"
echo ""

# Save current branch
ORIGINAL_BRANCH=$(git branch --show-current)
echo "Original branch: $ORIGINAL_BRANCH"
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
  if git fetch origin pull/$pr_num/head:test-pr-$pr_num 2>&1 | head -5; then
    echo "✓ PR branch fetched"
  else
    echo "✗ Failed to fetch PR"
    continue
  fi
  
  git checkout test-pr-$pr_num
  echo ""
  
  # Run fix-patches
  echo "Running fix-patches (SKIP_VALIDATION=true)..."
  start_time=$(date +%s)
  
  if SKIP_VALIDATION=true "$TOOL_PATH" fix-patches \
    --project "$project" \
    --pr "$pr_num" \
    --max-attempts 3 \
    --verbosity 6 \
    2>&1 | tee "$TEST_RESULTS_DIR/pr-$pr_num.log"; then
    exit_code=0
  else
    exit_code=$?
  fi
  
  end_time=$(date +%s)
  duration=$((end_time - start_time))
  
  # Collect results
  echo ""
  echo "Results for PR #$pr_num:"
  echo "  Exit code: $exit_code"
  echo "  Duration: ${duration}s"
  echo "  Log: $TEST_RESULTS_DIR/pr-$pr_num.log"
  
  # Extract metrics from log
  if grep -q "Patch fix successful" "$TEST_RESULTS_DIR/pr-$pr_num.log"; then
    status="✅ SUCCESS"
    attempts=$(grep -c "Starting fix attempt" "$TEST_RESULTS_DIR/pr-$pr_num.log" || echo "?")
    cost=$(grep "total_cost" "$TEST_RESULTS_DIR/pr-$pr_num.log" | tail -1 | grep -oE '\$[0-9.]+' || echo "?")
    tokens=$(grep "input_tokens\|output_tokens" "$TEST_RESULTS_DIR/pr-$pr_num.log" | tail -2 | tr '\n' ' ' || echo "?")
  else
    status="❌ FAILED"
    attempts="?"
    cost="?"
    tokens="?"
  fi
  
  echo "  Status: $status"
  echo "  Attempts: $attempts"
  echo "  Cost: $cost"
  echo "  Tokens: $tokens"
  
  # Check if patches were modified
  if git diff --quiet; then
    echo "  Changes: None"
  else
    echo "  Changes: Yes"
    git diff --stat > "$TEST_RESULTS_DIR/pr-$pr_num-diff.txt"
    git diff > "$TEST_RESULTS_DIR/pr-$pr_num-full-diff.txt"
    echo "  Diff saved: $TEST_RESULTS_DIR/pr-$pr_num-diff.txt"
  fi
  
  # Save metrics to CSV
  echo "$pr_num,$project,$expected_complexity,$status,$attempts,$duration,$cost,$tokens" >> "$TEST_RESULTS_DIR/metrics.csv"
  
  echo ""
  
  # Reset for next test
  git reset --hard HEAD
  git clean -fd
  git checkout "$ORIGINAL_BRANCH"
  
  # Pause between tests
  sleep 2
done

echo "=========================================="
echo "Testing complete!"
echo "=========================================="
echo ""
echo "Results directory: $TEST_RESULTS_DIR"
echo ""
echo "Files created:"
ls -lh "$TEST_RESULTS_DIR"
echo ""
echo "Metrics summary:"
if [ -f "$TEST_RESULTS_DIR/metrics.csv" ]; then
  echo "PR,Project,Complexity,Status,Attempts,Duration,Cost,Tokens"
  cat "$TEST_RESULTS_DIR/metrics.csv"
fi
echo ""
echo "To review detailed logs:"
echo "  cat $TEST_RESULTS_DIR/pr-XXXX.log"
echo ""
echo "To see what changed:"
echo "  cat $TEST_RESULTS_DIR/pr-XXXX-diff.txt"
