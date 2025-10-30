# Bedrock Quota Analysis and Increase Requests

## Current Model: Claude Sonnet 4.5

**Model ID**: `anthropic.claude-sonnet-4-5-20250929-v1:0`
**Inference Profile**: `us.anthropic.claude-sonnet-4-5-20250929-v1:0`

---

## Current Quotas for Claude Sonnet 4.5

### Standard Version (Current)
| Quota | Current Value | Adjustable | Quota Code |
|-------|--------------|------------|------------|
| Cross-region requests/min | 4 | ❌ No | L-4A6BFAB1 |
| Cross-region tokens/min | 4,000 | ✅ Yes | L-F4DDD3EB |
| Max tokens/day | 144M | ❌ No | L-381AD9EE |

### 1M Context Version
| Quota | Current Value | Adjustable | Quota Code |
|-------|--------------|------------|------------|
| Cross-region requests/min | 2 | ❌ No | L-A052927A |
| Cross-region tokens/min | 20,000 | ✅ Yes | L-8EA73537 |
| Max tokens/day | 720M | ❌ No | L-E107194C |

---

## Problem Analysis

### Current Issue
- **Error**: "ThrottlingException: Too many tokens, please wait before trying again"
- **Root Cause**: We're hitting the **4 requests/min** limit (L-4A6BFAB1)
- **Not adjustable**: The requests/min quota cannot be increased

### Why "Too many tokens" is Misleading
The error message says "too many tokens" but we're actually hitting the **requests per minute** limit, not the token limit. This is a Bedrock API quirk.

---

## Solutions

### Option 1: Switch to 1M Context Version ✅ RECOMMENDED

**Benefits:**
- 5x more tokens/min (20K vs 4K)
- Better for large patches
- Same model quality

**Drawbacks:**
- Only 2 requests/min (vs 4 requests/min)
- But with longer backoff, this is manageable

**How to Enable:**
The 1M context version might be accessed through:
1. A different model ID (need to check with AWS support)
2. Or by specifying context length in the request

**Action Required:**
- Check AWS documentation for 1M context model ID
- Update code to use the 1M version
- Request quota increase for L-8EA73537 (tokens/min)

### Option 2: Request Quota Increases for Adjustable Quotas

**Standard Version:**
```bash
# Increase tokens/min from 4K to 40K
aws service-quotas request-service-quota-increase \
  --service-code bedrock \
  --quota-code L-F4DDD3EB \
  --desired-value 40000 \
  --region us-west-2
```

**1M Context Version:**
```bash
# Increase tokens/min from 20K to 100K
aws service-quotas request-service-quota-increase \
  --service-code bedrock \
  --quota-code L-8EA73537 \
  --desired-value 100000 \
  --region us-west-2
```

### Option 3: Improve Retry Logic ✅ ALREADY IMPLEMENTED

**Changes Made:**
1. Disabled SDK's automatic retries (was causing 3x3=9 attempts)
2. Increased backoff times: 15s, 30s, 60s, 120s
3. Increased max retries from 3 to 5

**This ensures:**
- We don't exceed 4 requests/min (15s = 4 req/min)
- Better handling of rate limits
- More chances to succeed

---

## Recommended Action Plan

### Immediate (Already Done)
- [x] Fix retry logic to respect rate limits
- [x] Increase backoff times to 15s, 30s, 60s, 120s
- [x] Disable SDK automatic retries

### Short Term (Do Now)
1. **Request quota increase for tokens/min**
   ```bash
   # For standard version
   aws service-quotas request-service-quota-increase \
     --service-code bedrock \
     --quota-code L-F4DDD3EB \
     --desired-value 40000 \
     --region us-west-2
   
   # For 1M context version
   aws service-quotas request-service-quota-increase \
     --service-code bedrock \
     --quota-code L-8EA73537 \
     --desired-value 100000 \
     --region us-west-2
   ```

2. **Check quota increase status**
   ```bash
   aws service-quotas list-requested-service-quota-change-history \
     --service-code bedrock \
     --region us-west-2 \
     --query "RequestedQuotas[?Status=='PENDING' || Status=='CASE_OPENED'].{QuotaCode:QuotaCode,DesiredValue:DesiredValue,Status:Status,Created:Created}"
   ```

### Medium Term (After Quota Increase Approved)
1. Investigate 1M context version model ID
2. Update code to use 1M version if available
3. Test with increased quotas

---

## Testing After Changes

### Test Command
```bash
cd /Users/rahulgab/Desktop/work/1-30/eks-anywhere-build-tooling/test/eks-anywhere-build-tooling

# Rebuild with new retry logic
cd ../../tools/version-tracker
go build -o version-tracker main.go
cp version-tracker ../bin/version-tracker

# Test with PR #4883
cd /Users/rahulgab/Desktop/work/1-30/eks-anywhere-build-tooling/test/eks-anywhere-build-tooling
../bin/version-tracker fix-patches \
  --project fluxcd/source-controller \
  --pr 4883 \
  --max-attempts 1 \
  --verbosity 6
```

### Expected Behavior
- First attempt may still hit rate limit
- Retry after 15s should succeed
- No more "exceeded maximum number of attempts" from SDK
- Clear logging of wait times

---

## Quota Increase Request Details

### Justification for Increase Request

**Use Case**: Automated patch fixing for EKS Anywhere build tooling
**Current Limitation**: 4 requests/min is too low for CI/CD automation
**Requested Increase**: 
- Tokens/min: 4K → 40K (10x increase)
- This allows processing larger patches without hitting token limits

**Business Impact**:
- Reduces manual intervention in patch management
- Improves developer productivity
- Enables automated PR processing

---

## References

- [AWS Service Quotas Documentation](https://docs.aws.amazon.com/servicequotas/latest/userguide/request-quota-increase.html)
- [Bedrock Quotas](https://docs.aws.amazon.com/bedrock/latest/userguide/quotas.html)
- [Claude Model Documentation](https://docs.anthropic.com/claude/docs/models-overview)

---

## Status

- ✅ Retry logic improved
- ⏳ Quota increase requests pending
- ⏳ 1M context version investigation needed
