package creatorroom

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeRealtime struct {
	created   []ChannelCreateRequest
	deleted   []string
	published []PublishRequest
	keys      []string
	tokenReq  []TokenIssueRequest
}

func (f *fakeRealtime) RealtimeChannelDelete(ctx context.Context, channel string) error {
	f.deleted = append(f.deleted, channel)
	return nil
}

func (f *fakeRealtime) RealtimeChannelCreate(ctx context.Context, req ChannelCreateRequest) (*createChannelResponse, error) {
	f.created = append(f.created, req)
	return &createChannelResponse{Channel: req.Channel}, nil
}

func (f *fakeRealtime) RealtimePublish(ctx context.Context, req PublishRequest, idempotencyKey string) error {
	f.published = append(f.published, req)
	f.keys = append(f.keys, idempotencyKey)
	return nil
}

func (f *fakeRealtime) RealtimeTokenIssue(ctx context.Context, req TokenIssueRequest) (*issueTokenResponse, error) {
	f.tokenReq = append(f.tokenReq, req)
	return &issueTokenResponse{Token: "tok_demo"}, nil
}

func TestDecideRoomEvents(t *testing.T) {
	tests := []struct {
		name  string
		input SyncRequest
		want  []string
		count int
	}{
		{
			name: "paid and ready publishes subscriber and asset events",
			input: SyncRequest{
				CreatorID:       "creator_42",
				RoomID:          "drops",
				SubscriberID:    "sub_9",
				AssetID:         "asset_gold_pack",
				PurchaseState:   "paid",
				ProcessingState: "ready",
				AccountID:       "acct_7",
			},
			want:  []string{"subscriber.updated", "asset.ready"},
			count: 2,
		},
		{
			name: "paid but processing not ready publishes subscriber update only",
			input: SyncRequest{
				CreatorID:       "creator_42",
				RoomID:          "drops",
				SubscriberID:    "sub_9",
				AssetID:         "asset_gold_pack",
				PurchaseState:   "paid",
				ProcessingState: "processing",
				AccountID:       "acct_7",
			},
			want:  []string{"subscriber.updated"},
			count: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRealtime{}
			service := NewService(fake)

			resp, err := service.SyncPurchase(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("SyncPurchase() error = %v", err)
			}

			if resp.PublishedEvents != tt.count {
				t.Fatalf("PublishedEvents = %d, want %d", resp.PublishedEvents, tt.count)
			}
			if len(fake.deleted) != 1 || fake.deleted[0] != resp.Channel {
				t.Fatalf("deleted channels = %v, want [%s]", fake.deleted, resp.Channel)
			}
			for i, name := range tt.want {
				if resp.Events[i] != name {
					t.Fatalf("Events[%d] = %s, want %s", i, resp.Events[i], name)
				}
				if fake.published[i].Event != name {
					t.Fatalf("published event[%d] = %s, want %s", i, fake.published[i].Event, name)
				}
			}
		})
	}
}

func TestIssueSubscriberTokenScopesChannel(t *testing.T) {
	fake := &fakeRealtime{}
	service := NewService(fake)

	resp, err := service.IssueSubscriberToken(context.Background(), TokenRequest{
		CreatorID:    "creator_42",
		RoomID:       "drops",
		SubscriberID: "sub_9",
	})
	if err != nil {
		t.Fatalf("IssueSubscriberToken() error = %v", err)
	}

	if resp.Channel != "creator.creator_42.room.drops" {
		t.Fatalf("Channel = %s", resp.Channel)
	}
	if len(fake.tokenReq) != 1 {
		t.Fatalf("token requests = %d, want 1", len(fake.tokenReq))
	}
	if fake.tokenReq[0].ClientID != "subscriber:sub_9" {
		t.Fatalf("ClientID = %s", fake.tokenReq[0].ClientID)
	}
}

func TestSyncPurchasePublishesJSONPayload(t *testing.T) {
	fake := &fakeRealtime{}
	service := NewService(fake)

	_, err := service.SyncPurchase(context.Background(), SyncRequest{
		CreatorID:       "creator_42",
		RoomID:          "drops",
		SubscriberID:    "sub_9",
		AssetID:         "asset_gold_pack",
		PurchaseState:   "paid",
		ProcessingState: "ready",
		AccountID:       "acct_7",
	})
	if err != nil {
		t.Fatalf("SyncPurchase() error = %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(fake.published[1].Data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["asset_id"] != "asset_gold_pack" {
		t.Fatalf("asset_id = %s", payload["asset_id"])
	}
	if fake.keys[1] != "creator_42:drops:sub_9:asset.ready" {
		t.Fatalf("idempotency key = %s", fake.keys[1])
	}
}
