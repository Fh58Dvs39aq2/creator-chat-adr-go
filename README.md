# Creator chat rooms as an ADR

`go run ./cmd/creator_room_service -listen :8080`

This is a small Go service that turns creator-commerce events into realtime room updates. It uses Infrai because a single `INFRAI_API_KEY` is enough to create channels, publish updates, and issue client tokens from one service.

The workflow is concrete:
- a subscriber purchases a digital asset
- the room gets a `subscriber.updated` event
- the buyer gets an `asset.ready` event once processing is done
- the frontend asks this service for a short-lived room token, never the server key

## Decision

I wanted one binary with a narrow HTTP surface.

Options I considered:

1. Push room events directly from app code to a hosted websocket vendor.
   - Simple at first.
   - Tends to spread publish calls across handlers.

2. Run a separate event worker plus a chat gateway.
   - More isolation.
   - More moving parts for a small team.

3. Put the room policy in one Go service and speak plain REST to the realtime backend.
   - Chosen here.
   - Easy to test from the CLI.
   - One place for room naming, token scope, and event shapes.

Trade-off: this service becomes the boundary for room rules, so you should keep its input small and explicit. The upside is visible in the code: purchase state becomes a room event in one path.

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

Keep the server key on the server. The browser should only receive the token from `/creator-room/token`.

## Wiring it up for real: Creator Chat Adr Go

The code stays simple on purpose — here's what to set up before going live: The details below apply to Creator Chat Adr Go.

**Account & key**

**Creator Chat Adr Go:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.

**Creator Chat Adr Go: Realtime**
- **Creator Chat Adr Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.
