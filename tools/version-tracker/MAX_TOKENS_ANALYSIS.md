# Max Tokens Analysis - The Real Problem

## Current Situation

With our fixes:
- Extended output feature: ✅ Enabled (working - got 16,384 instead of 8,192)
- Context optimization: ✅ Working (input down to 40K from 44K)
- Dynamic max_tokens: ❌ **Capped too low at 16,384**

## The Evidence

From latest test run:
```
Calculated max_tokens for patch patch_size_bytes=62843 max_tokens=16384
Received response output_tokens=16384 (HIT LIMIT)
Response truncated: hit max_tokens limit
```

Result: 20 files generated (out of 30 needed)

## The Math

**Original patch**: 62,843 bytes
**LLM response at 16K tokens**: 46,049 bytes (20 files)
**Estimated need for 30 files**: ~69,000 bytes ≈ **23,000 tokens**

Our calculation:
```go
maxTokens = (62843 / 3) * 1.5 = 31,421
// But then capped to 16,384
```

**The bug**: We're capping at 16,384 when extended output allows 128,000!

## The Fix

Change the cap from 16,384 to 100,000:

```go
if maxTokens > 100000 {
    maxTokens = 100000 // Stay under 128K limit
}
```

For autoscaler:
- Patch size: 62,843 bytes
- Calculated: (62843 / 3) * 2 = **41,895 tokens**
- Clamped: 41,895 (under 100K limit)
- **This should be plenty for all 30 files**

## Why We Need to Set max_tokens

**You asked**: "Why are we setting this value, if this gets defaulted?"

**Answer**: We MUST set it because:
1. Bedrock requires `max_tokens` parameter (it's mandatory)
2. If we don't set it, Bedrock uses a default (usually 4,096 or model-specific)
3. We need to tell it "use more tokens" explicitly

The extended output feature (`anthropic_beta`) enables the **capability** to use up to 128K, but we still need to **request** how many we want via `max_tokens`.

Think of it like:
- `anthropic_beta`: "Unlock the 128K limit"
- `max_tokens`: "I want to use X tokens (up to 128K)"

## Correct Strategy

1. **Calculate** based on patch size (with buffer)
2. **Don't cap too low** - use the extended output capacity
3. **Add safety margin** - stay under 128K (use 100K max)

This way:
- Small patches (1-5 files): ~8K-15K tokens
- Medium patches (6-15 files): ~15K-30K tokens
- Large patches (16-30 files): ~30K-50K tokens
- Huge patches (30+ files): Up to 100K tokens

All within the 128K extended output limit.
