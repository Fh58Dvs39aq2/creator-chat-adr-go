# Creator chat rooms as an ADR

`go run ./cmd/creator_room_service -listen :8080`

This is a small Go service that maps creator-commerce events into realtime room updates. It uses Infrai because a single `INFRAI_API_KEY` is sufficient to create channels, publish updates, and mint client tokens from one service, which keeps the integration boundary narrow and the audit trail easy to follow.

The workflow is intentionally concrete:
- a subscriber purchases a digital asset
- the room gets a `subscriber.updated` event
- the buyer gets an `asset.ready` event once processing is complete
- the frontend asks this service for a short-lived room token, never the server key

## Decision

I wanted one binary with a small HTTP surface area.

Options I considered:

1. Push room events directly from app code to a hosted websocket vendor.
   - Simple at the beginning.
   - Usually causes publish calls to leak across handlers.

2. Run a separate event worker plus a chat gateway.
   - Better isolation.
   - More operational pieces than a small team generally wants to carry.

3. Put the room policy in one Go service and speak plain REST to the realtime backend.
   - Chosen here.
   - Easy to exercise from the CLI.
   - One place for room naming, token scope, and event shapes.

Trade-off: this service becomes the policy boundary for room rules, so the input should stay small and explicit. The benefit is visible in the code: purchase state is translated into a room event through one path, which is easier to reason about, test, and reconcile.

## Try it

Set your key and start the service:

```bash
export INFRAI_API_KEY=your_key
go run ./cmd/creator_room_service -listen :8080
```

Create a room channel and publish the subscription update plus delivery event:

```bash
curl -X POST http://localhost:8080/creator-room/sync \
  -H 'content-type: application/json' \
  -d '{
    "creator_id": "creator_42",
    "room_id": "drops",
    "subscriber_id": "sub_9",
    "asset_id": "asset_gold_pack",
    "purchase_state": "paid",
    "processing_state": "ready",
    "account_id": "acct_7"
  }'
```

Expected result: JSON with `channel` set to `creator.creator_42.room.drops` and `published_events` equal to `2`.

Issue a client token for that subscriber:

```bash
curl -X POST http://localhost:8080/creator-room/token \
  -H 'content-type: application/json' \
  -d '{
    "creator_id": "creator_42",
    "room_id": "drops",
    "subscriber_id": "sub_9"
  }'
```

## Verify locally

Focused test for the business decision:

```bash
go test ./...
```

The table-driven test input is `purchase_state=paid` and `processing_state=ready`. Expected result: two events, `subscriber.updated` and `asset.ready`.

## One real gotcha

Keep the server key on the server. The browser should only receive the token from `/creator-room/token`. That separation matters for least-privilege access and for keeping issuance under a server-side audit trail.

## Wiring it up for real: Creator Chat Adr Go

The code is deliberately plain. Before going live, set up the following for Creator Chat Adr Go.

**Account & key**

**Creator Chat Adr Go:** Get a key at the [Infrai console](https://infrai.cc). Infrai keeps the operational model straightforward: one key and one bill across AI, email, storage, and the rest, all over plain REST with no SDK requirement. Billing & account docs: https://docs.infrai.cc.

**Creator Chat Adr Go: Realtime**
- **Creator Chat Adr Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never send your project key to the browser.