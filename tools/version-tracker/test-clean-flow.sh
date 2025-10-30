#!/bin/bash
set -e

echo "=== Testing Clean Flow Implementation ==="
echo ""

# Configuration
REPO_DIR=$(git rev-parse --show-toplevel)
PROJECT="fluxcd/source-controller"
PR="4883"

echo "1. Building version-tracker..."
cd "$REPO_DIR/tools/version-tracker"
go build -o ../../bin/version-tracker .
echo "✓ Build complete"
echo ""

echo "2. Cleaning up any previous test state..."
cd "$REPO_DIR/test/eks-anywhere-build-tooling"
PROJECT_PATH="projects/$PROJECT"

# Remove the cloned repo to ensure clean state
if [ -d "$PROJECT_PATH/source-controller" ]; then
    echo "  Removing existing source-controller directory..."
    rm -rf "$PROJECT_PATH/source-controller"
fi

# Remove any marker files
rm -f "$PROJECT_PATH/source-controller/eks-anywhere-checkout-"*

echo "✓ Cleanup complete"
echo ""

echo "3. Running fix-patches with clean state..."
cd "$REPO_DIR/test/eks-anywhere-build-tooling"

SKIP_VALIDATION=true "$REPO_DIR/bin/version-tracker" fix-patches \
    --project "$PROJECT" \
    --pr "$PR" \
    --max-attempts 3 \
    --verbosity 6 \
    2>&1 | tee "$REPO_DIR/tools/version-tracker/test-clean-flow.log"

EXIT_CODE=$?

echo ""
echo "=== Test Complete ==="
echo "Exit code: $EXIT_CODE"
echo "Log saved to: tools/version-tracker/test-clean-flow.log"
echo ""

echo "4. Checking prompts for pollution..."
echo ""

if [ -f "/tmp/llm-prompt-attempt-1.txt" ]; then
    echo "Attempt 1 - Checking go.mod context..."
    if grep -A 5 "Current file content (around line 8):" /tmp/llm-prompt-attempt-1.txt | grep -q "replace github.com/sigstore/timestamp-authority"; then
        echo "  ❌ POLLUTED: timestamp-authority line already present"
    else
        echo "  ✅ CLEAN: timestamp-authority line NOT present"
    fi
    echo ""
fi

if [ -f "/tmp/llm-prompt-attempt-2.txt" ]; then
    echo "Attempt 2 - Checking for dynamic context..."
    if grep -q "Applied patch go.mod cleanly" /tmp/llm-prompt-attempt-2.txt; then
        echo "  ✅ Shows go.mod succeeded in attempt 1"
    else
        echo "  ❌ Does not show go.mod success"
    fi
    
    if grep -q "FAILED (needs fixing): go.mod, go.sum" /tmp/llm-prompt-attempt-2.txt; then
        echo "  ❌ Still shows both files as failed (static context)"
    else
        echo "  ✅ Shows updated status (dynamic context)"
    fi
    echo ""
fi

echo "5. To review prompts manually:"
echo "  cat /tmp/llm-prompt-attempt-1.txt"
echo "  cat /tmp/llm-prompt-attempt-2.txt"
echo ""

exit $EXIT_CODE
