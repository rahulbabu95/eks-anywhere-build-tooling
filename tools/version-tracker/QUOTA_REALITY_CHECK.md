# Bedrock Quota Reality Check

## ❌ There is NO 1M Context Separate Model

After investigation, here's the truth:

### The Reality
- **Same Model ID**: `anthropic.claude-sonnet-4-5-20250929-v1:0`
- **Same Inference Profile**: `us.anthropic.claude-sonnet-4-5-20250929-v1:0`
- **Different Quotas**: The "1M context" refers to QUOTA limits, not a different model

### What the Quotas Mean

| Quota Name | What It Actually Means |
|------------|------------------------|
| "Sonnet 4.5 V1" | Standard quota tier for this model |
| "Sonnet 4.5 V1 1M Context Length" | Higher quota tier for the SAME model |

**They're not different models - they're different quota tiers!**

---

## The Real Problem

### Current Limits (ACTIVE)
```
Cross-region requests/min: 4 req/min (NOT ADJUSTABLE)
Cross-region tokens/min: 4,000 tokens/min (adjustable, but low)
```

### Requested Limits (PENDING APPROVAL)
```
Standard tier: 200,000 tokens/min
1M tier: 1,000,000 tokens/min
```

### The Bottleneck
**We're hitting the 4 requests/min limit**, which CANNOT be increased!

The "Too many tokens" error is **misleading** - it's actually a rate limit on requests, not tokens.

---

## Why We Keep Failing

### The Math
- 4 requests/min = 1 request every 15 seconds
- Our retry logic: 15s, 30s, 60s, 120s
- But if we make a request at T=0, then retry at T=15s, we've made 2 requests in 15s = 8 req/min!

### The Fix Needed
We need to track the LAST request time and ensure 15s has passed since then.

---

## Solution: Add Request Throttling

We need to add a global rate limiter that ensures we never exceed 4 req/min:

```go
var (
    lastRequestTime time.Time
    requestMutex    sync.Mutex
)

func waitForRateLimit() {
    requestMutex.Lock()
    defer requestMutex.Unlock()
    
    // Ensure at least 15 seconds since last request (4 req/min = 15s between requests)
    timeSinceLastRequest := time.Since(lastRequestTime)
    if timeSinceLastRequest < 15*time.Second {
        waitTime := 15*time.Second - timeSinceLastRequest
        logger.Info("Rate limiting: waiting before next request", "wait_seconds", waitTime.Seconds())
        time.Sleep(waitTime)
    }
    
    lastRequestTime = time.Now()
}
```

Then call this BEFORE every Bedrock API call.

---

## Current Status

### Quota Requests
```
Status: CASE_OPENED (being reviewed by AWS)
Expected: 1-2 business days for approval
```

### What Happens After Approval
- ✅ Token limits increase dramatically (200K or 1M tokens/min)
- ❌ Request limits stay the same (4 req/min)
- ✅ We can process LARGER patches without token errors
- ❌ We still need to respect the 4 req/min limit

---

## Immediate Action Required

### 1. Add Rate Limiting to Code
Implement the `waitForRateLimit()` function above and call it before every API request.

### 2. Test with Rate Limiting
The retry logic alone isn't enough - we need explicit rate limiting.

### 3. Wait for Quota Approval
Once approved, we'll have more token headroom, but still need rate limiting.

---

## Long-Term Solution

### Option A: Use Provisioned Throughput
- Pay for dedicated capacity
- Get higher request limits
- More expensive but more reliable

### Option B: Batch Processing
- Queue multiple patches
- Process them with proper spacing
- More efficient use of rate limits

### Option C: Use Multiple Regions
- Spread requests across regions
- Each region has separate quotas
- More complex but higher throughput

---

## Bottom Line

**The "1M context" is NOT a different model - it's just a higher quota tier for the same model.**

**The real bottleneck is the 4 requests/min limit, which cannot be increased without provisioned throughput.**

**We MUST add explicit rate limiting to the code to respect this limit.**

---

## Next Steps

1. ✅ Understand there's no separate 1M model
2. ⏳ Add rate limiting to code
3. ⏳ Wait for quota approval (1-2 days)
4. ⏳ Test with increased token quotas
5. ⏳ Consider provisioned throughput if needed
