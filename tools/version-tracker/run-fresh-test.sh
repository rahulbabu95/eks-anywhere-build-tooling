#!/bin/bash

set -e

echo "=== Fresh Test Run with Enhanced Context ==="
echo

# Clean up old artifacts
echo "1. Cleaning up old test artifacts..."
rm -f /tmp/llm-prompt-attempt-*.txt
rm -f /tmp/llm-response-attempt-*.txt
find test/eks-anywhere-build-tooling/projects -name ".llm-patch-debug.txt" -delete 2>/dev/null || true
find test/eks-anywhere-build-tooling/projects -name "*.rej" -delete 2>/dev/null || true

# Reset test repo
echo "2. Resetting test repository..."
cd test/eks-anywhere-build-tooling/projects/fluxcd/source-controller
if [ -d "source-controller" ]; then
    cd source-controller
    git reset --hard HEAD 2>/dev/null || true
    git clean -fd 2>/dev/null || true
    cd ..
fi
cd ../../../../..

# Run the test
echo "3. Running patch fixer with enhanced context..."
echo
cd test/eks-anywhere-build-tooling

./tools/version-tracker/bin/version-tracker fix-patches \
    --project fluxcd/source-controller \
    --pr 4883 \
    --max-attempts 3 \
    2>&1 | tee auto-patch-source-controller-$(date +%Y%m%d-%H%M%S).log

echo
echo "=== Test Complete ==="
echo

# Check for debug files
echo "4. Checking for debug files..."
if [ -f "/tmp/llm-prompt-attempt-1.txt" ]; then
    echo "✅ Prompt file created: /tmp/llm-prompt-attempt-1.txt"
    echo "   Size: $(wc -c < /tmp/llm-prompt-attempt-1.txt) bytes"
else
    echo "❌ Prompt file NOT created"
fi

if [ -f "/tmp/llm-response-attempt-1.txt" ]; then
    echo "✅ Response file created: /tmp/llm-response-attempt-1.txt"
    echo "   Size: $(wc -c < /tmp/llm-response-attempt-1.txt) bytes"
else
    echo "❌ Response file NOT created"
fi

if [ -f "projects/fluxcd/source-controller/.llm-patch-debug.txt" ]; then
    echo "✅ Debug patch created: projects/fluxcd/source-controller/.llm-patch-debug.txt"
    echo "   Size: $(wc -c < projects/fluxcd/source-controller/.llm-patch-debug.txt) bytes"
else
    echo "❌ Debug patch NOT created"
fi

echo
echo "5. Quick checks..."

# Check if Expected vs Actual section exists
if [ -f "/tmp/llm-prompt-attempt-1.txt" ]; then
    if grep -q "Expected vs Actual File State" /tmp/llm-prompt-attempt-1.txt; then
        echo "✅ Prompt has 'Expected vs Actual' section"
    else
        echo "❌ Prompt missing 'Expected vs Actual' section"
    fi
fi

# Check model used
if grep -q "claude-sonnet-4-5" auto-patch-source-controller-*.log 2>/dev/null; then
    echo "✅ Using Claude Sonnet 4.5"
elif grep -q "claude-3-7" auto-patch-source-controller-*.log 2>/dev/null; then
    echo "❌ Still using Claude 3.7 - binary not rebuilt?"
fi

echo
echo "=== Next Steps ==="
echo "1. View prompt: cat /tmp/llm-prompt-attempt-1.txt | less"
echo "2. View response: cat /tmp/llm-response-attempt-1.txt | less"
echo "3. View patch: cat projects/fluxcd/source-controller/.llm-patch-debug.txt"
echo "4. Check for 'Expected vs Actual': grep -A 30 'Expected vs Actual' /tmp/llm-prompt-attempt-1.txt"
