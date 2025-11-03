# Bedrock Limits for Your Account - Claude Sonnet 4.5

## Model: anthropic.claude-sonnet-4-5-20250929-v1:0

### Cross-Region Inference Limits (What We're Using)

**Requests per minute**:
- Quota Code: `L-4A6BFAB1`
- Limit: **200 requests/minute**
- Current usage: Using inference profile `us.anthropic.claude-sonnet-4-5-20250929-v1:0`

**Tokens per minute**:
- Quota Code: `L-F4DDD3EB`
- Limit: **200,000 tokens/minute**
- This is TOTAL tokens (input + output combined)

**Tokens per day**:
- Quota Code: `L-381AD9EE`
- Limit: **144,000,000 tokens/day** (144M)
- This is for cross-region calls (doubled from base)

**Global cross-region tokens per minute**:
- Quota Code: `L-27477D78`
- Limit: **500,000 tokens/minute**

**Global cross-region tokens per day**:
- Quota Code: `L-BC182137`
- Limit: **720,000,000 tokens/day** (720M)

### What This Means for Our Use Case

#### Current Usage Per Patch (Autoscaler)
- Input: ~10,000 tokens
- Output: ~23,000 tokens
- **Total: ~33,000 tokens per attempt**

#### Limits Check
✅ **Requests/min**: 200 limit, we do ~1 request every 15-20 seconds = **3-4 requests/min** (well under limit)

✅ **Tokens/min**: 200,000 limit, we use ~33,000 per request × 3-4 requests = **99,000-132,000 tokens/min** (under limit)

✅ **Tokens/day**: 144M limit, even with 1000 patches = **33M tokens** (well under limit)

### Output Token Limits

**This is the key question**: What's the max output tokens?

From the quotas, I don't see a specific "max output tokens" limit listed. This is controlled by:

1. **Model parameter**: `max_tokens` in the API request
2. **Model capability**: Claude's inherent limits
3. **Extended output feature**: `anthropic_beta` flag

### Extended Output Feature

According to AWS Bedrock docs:
- Feature: `output-128k-2025-02-19`
- Enables: **Up to 128,000 output tokens**
- Without this: Default is **8,192 output tokens**

**We're using this feature**, so we should be able to request up to 128K output tokens.

### Why We're Capping at 100K

In our code:
```go
if maxTokens > 100000 {
    maxTokens = 100000 // Stay well under 128K limit for safety
}
```

This is a **safety margin** to avoid hitting the absolute limit.

## Verification

Let me check if the extended output feature is actually working:

From your last test run (before logs were cleaned):
- We requested: 41,894 tokens
- We got: 16,384 tokens (truncated)

**This suggests the extended output feature might not be working!**

### Possible Issues

1. **Feature not enabled in your account** - Need to check with AWS
2. **Feature flag syntax wrong** - Need to verify
3. **Model doesn't support it** - Need to check model capabilities

## Action Items

### 1. Verify Extended Output is Enabled
```bash
# Check if the feature is available
aws bedrock get-foundation-model \
  --model-identifier anthropic.claude-sonnet-4-5-20250929-v1:0 \
  --query 'modelDetails.{customizationsSupported:customizationsSupported,inferenceTypesSupported:inferenceTypesSupported}'
```

### 2. Test with Simple Request
Create a test script that requests 20K output tokens and see if it works:
```go
requestBody := map[string]any{
    "anthropic_version": "bedrock-2023-05-31",
    "max_tokens":        20000,
    "anthropic_beta":    []string{"output-128k-2025-02-19"},
    "messages": []map[string]string{
        {"role": "user", "content": "Count from 1 to 1000, one number per line"},
    },
}
```

### 3. Check AWS Documentation
The extended output feature might:
- Require account enablement
- Have different syntax
- Not be available for cross-region inference profiles
- Be named differently

## Current Status

**We're hitting a 16,384 token output limit**, which suggests:
- Extended output feature is NOT working as expected
- OR there's a different limit for cross-region inference profiles
- OR the feature flag syntax is incorrect

**Need to investigate** why we're capped at 16K instead of 128K.
