# HostedAt v8go Worker Runtime — Bug Report

**Tested:** 2026-02-20 (updated after fixes)
**Test site:** https://rei-api-test.hostedat.ditto.moe
**CLI version:** v2.0.2
**Runtime:** V8 via v8go

---

## ~~Bug 1: KV `list()` metadata serialization~~ ✅ FIXED

Metadata now roundtrips correctly through `put()` → `list()`. Object metadata like `{tag: "test", num: 42}` is returned as a proper parsed object instead of `[object Object]`.

---

## ~~Bug 2: Gzip compress→decompress roundtrip~~ ✅ FIXED

CompressionStream("gzip") → DecompressionStream("gzip") roundtrip now works perfectly. 380 bytes in, 380 bytes out, exact match.

---

## ~~Bug 3: WebSocketPair in-process message delivery~~ ✅ FIXED

In-process `send()` now delivers directly to `_peer` via microtask queue when not HTTP-bridged. The previous fix had a too-strict peer readyState check (`>= 1`) that required both sides to call `accept()`. Changed to `< 2` so the client side (readyState 0/CONNECTING) can receive messages without calling `accept()`, matching Cloudflare Workers behavior. Also fixed `close()` peer notification with the same relaxed check, and added `Symbol.iterator` to WebSocketPair so `[client, server] = new WebSocketPair()` works.

---

## ~~Bug 4: MessageChannel/MessagePort~~ (Not a bug)

Working as designed — uses `queueMicrotask()` for delivery.

---

## ~~Bug 5: TCP `connect()` socket reads~~ ✅ FIXED

`reader.read()` now returns data correctly. Two fixes: (1) `__tcpRead` Go callback changed from single 5s wait to bounded retry loop (30 x 1s), ensuring data arriving after the first window is captured. (2) ReadableStream polyfill's `pull()` trigger now re-fires when `pull()` returns without enqueuing data and pending reads exist, preventing the read promise from hanging forever.

---

## ~~Bug 6: D1 and Durable Objects API endpoints~~ ✅ FIXED

Both endpoints now exist and work:
- `POST /api/v1/sites/:id/worker/d1` — creates D1 database binding
- `POST /api/v1/sites/:id/worker/do` — creates Durable Object binding
- D1 `prepare("SELECT 1").first()` executes correctly from worker code

---

## Summary

| # | Bug | Status | Notes |
|---|-----|--------|-------|
| 1 | KV list() metadata | ✅ FIXED | Metadata roundtrips correctly |
| 2 | Gzip roundtrip | ✅ FIXED | Compress→decompress matches |
| 3 | WebSocketPair in-process | ✅ FIXED | Relaxed peer readyState check + iterator |
| 4 | MessageChannel | N/A | Not a bug |
| 5 | TCP socket reads | ✅ FIXED | Bounded retry loop + pull() re-trigger |
| 6 | D1/DO API endpoints | ✅ FIXED | Both endpoints work |

**5 fixed, 0 remaining.** All reported bugs are resolved.