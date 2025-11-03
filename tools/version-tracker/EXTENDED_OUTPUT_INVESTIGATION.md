# Extended Output Feature Investigation

## The Mystery

We're requesting 41,894 tokens but only getting 16,384 tokens in the response.

## Evidence

From your last test run:
```
Calculated max_tokens for patch patch_size_bytes=62843 max_tokens=41894
Received response output_tokens=16384 (TRUNCATED)
```

## Hypothesis: Extended Output Not Working

The extended output feature (`output-128k-2025-02-19`) might not be:
1. Available for cross-region inference profiles
2. Enabled in your account
3. Using the correct syntax
4. Supported by this specific model version

## What We Know

### Your Account Limits
- ✅ Requests/min: 200 (plenty)
- ✅ Tokens/min: 200,000 (plenty)
- ✅ Tokens/day: 144M (plenty)

### Model Capabilities
- Model: `anthropic.claude-sonnet-4-5-20250929-v1:0`
- Inference type: `INFERENCE_PROFILE` (cross-region)
- Streaming: Supported
- Customizations: None listed

### The 16,384 Token Ceiling

This is **exactly 2× the standard 8,192 limit**, which suggests:
- Maybe extended output gives 2× boost (8K → 16K)?
- Or there's a different limit for inference profiles?
- Or the feature isn't fully enabled?

## Testing the Extended Output Feature

Let me create a simple test to verify:

```bash
# Test if extended output works
cat > /tmp/test-extended-output.json << 'EOF'
{
  "anthropic_version": "bedrock-2023-05-31",
  "max_tokens": 30000,
  "anthropic_beta": ["output-128k-2025-02-19"],
  "messages": [
    {
      "role": "user",
      "content": "Write a story that is exactly 20,000 tokens long. Keep writing until you reach that length."
    }
  ]
}
EOF

aws bedrock-runtime invoke-model \
  --model-id us.anthropic.claude-sonnet-4-5-20250929-v1:0 \
  --body file:///tmp/test-extended-output.json \
  --region us-west-2 \
  /tmp/test-output.json

# Check how many tokens we got
cat /tmp/test-output.json | jq '.usage.output_tokens'
```

If this returns ~20K tokens, extended output works.
If it returns 8K or 16K, it doesn't.

## Alternative Explanation

Maybe the **inference profile** has different limits than the base model?

Cross-region inference profiles might have:
- Lower output limits for reliability
- Different feature support
- Regional variations

## Recommendation

**Before implementing the autoscaler special case**, let's verify:

1. **Test extended output** with the script above
2. **Check if 16K is the actual limit** for cross-region profiles
3. **Consider using base model** instead of inference profile (if that's the issue)

If extended output truly doesn't work or caps at 16K, then:
- **Autoscaler special case is necessary** (patch needs 23K tokens)
- **Or we need to chunk the patch** into multiple LLM calls
- **Or we need to use a different model/approach**

## Next Steps

1. Run the test script to verify extended output
2. If it works (gets >16K tokens), debug why our code isn't getting it
3. If it doesn't work (caps at 16K), implement autoscaler special case
4. Document the actual limits for future reference
