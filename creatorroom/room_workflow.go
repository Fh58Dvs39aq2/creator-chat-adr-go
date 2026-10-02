package creatorroom

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type Service struct {
	infrai realtimeAPI
}

type realtimeAPI interface {
	RealtimeChannelCreate(ctx context.Context, req ChannelCreateRequest) (*createChannelResponse, error)
	RealtimeChannelDelete(ctx context.Context, channel string) error
	RealtimePublish(ctx context.Context, req PublishRequest, idempotencyKey string) error
	RealtimeTokenIssue(ctx context.Context, req TokenIssueRequest) (*issueTokenResponse, error)
}

type InputError struct {
	Message string
}

func (e InputError) Error() string { return e.Message }

type SyncRequest struct {
	CreatorID       string `json:"creator_id"`
	RoomID          string `json:"room_id"`
	SubscriberID    string `json:"subscriber_id"`
	AssetID         string `json:"asset_id"`
	PurchaseState   string `json:"purchase_state"`
	ProcessingState string `json:"processing_state"`
	AccountID       string `json:"account_id"`
}

type SyncResponse struct {
	Channel         string   `json:"channel"`
	PublishedEvents int      `json:"published_events"`
	Events          []string `json:"events"`
}

type TokenRequest struct {
	CreatorID    string `json:"creator_id"`
	RoomID       string `json:"room_id"`
	SubscriberID string `json:"subscriber_id"`
}

type TokenResponse struct {
	Channel  string `json:"channel"`
	ClientID string `json:"client_id"`
	Token    string `json:"token"`
}

type roomEvent struct {
	Name string
	Data map[string]string
}

func NewService(infrai realtimeAPI) *Service {
	return &Service{infrai: infrai}
}

func (s *Service) SyncPurchase(ctx context.Context, req SyncRequest) (response *SyncResponse, err error) {
	if strings.TrimSpace(req.CreatorID) == "" || strings.TrimSpace(req.RoomID) == "" || strings.TrimSpace(req.SubscriberID) == "" || strings.TrimSpace(req.AssetID) == "" {
		return nil, InputError{Message: "creator_id, room_id, subscriber_id, and asset_id are required"}
	}

	channel := roomChannel(req.CreatorID, req.RoomID)
	if _, err := s.infrai.RealtimeChannelCreate(ctx, ChannelCreateRequest{
		Channel: channel,
		Type:    "presence",
		Vendor:  "ably",
	}); err != nil {
		return nil, err
	}
	defer func() {
		if deleteErr := s.infrai.RealtimeChannelDelete(ctx, channel); deleteErr != nil {
			err = fmt.Errorf("delete channel %s: %w", channel, deleteErr)
			response = nil
		}
	}()

	events := decideRoomEvents(req)
	published := make([]string, 0, len(events))
	for _, evt := range events {
		payload, err := json.Marshal(evt.Data)
		if err != nil {
			return nil, err
		}
		if err := s.infrai.RealtimePublish(ctx, PublishRequest{
			Channel:   channel,
			Event:     evt.Name,
			Data:      payload,
			AccountID: req.AccountID,
		}, publishKey(req, evt.Name)); err != nil {
			return nil, err
		}
		published = append(published, evt.Name)
	}

	return &SyncResponse{
		Channel:         channel,
		PublishedEvents: len(published),
		Events:          published,
	}, nil
}

func (s *Service) IssueSubscriberToken(ctx context.Context, req TokenRequest) (*TokenResponse, error) {
	if strings.TrimSpace(req.CreatorID) == "" || strings.TrimSpace(req.RoomID) == "" || strings.TrimSpace(req.SubscriberID) == "" {
		return nil, InputError{Message: "creator_id, room_id, and subscriber_id are required"}
	}

	channel := roomChannel(req.CreatorID, req.RoomID)
	clientID := "subscriber:" + req.SubscriberID
	issued, err := s.infrai.RealtimeTokenIssue(ctx, TokenIssueRequest{
		ClientID:     clientID,
		Channels:     []string{channel},
		Capabilities: []string{"subscribe", "presence"},
		TTLSeconds:   900,
	})
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		Channel:  channel,
		ClientID: clientID,
		Token:    issued.Token,
	}, nil
}

func decideRoomEvents(req SyncRequest) []roomEvent {
	events := []roomEvent{}
	if req.PurchaseState == "paid" {
		events = append(events, roomEvent{
			Name: "subscriber.updated",
			Data: map[string]string{
				"subscriber_id":  req.SubscriberID,
				"asset_id":       req.AssetID,
				"purchase_state": req.PurchaseState,
			},
		})
	}
	if req.PurchaseState == "paid" && req.ProcessingState == "ready" {
		events = append(events, roomEvent{
			Name: "asset.ready",
			Data: map[string]string{
				"subscriber_id":    req.SubscriberID,
				"asset_id":         req.AssetID,
				"processing_state": req.ProcessingState,
			},
		})
	}
	return events
}

func roomChannel(creatorID, roomID string) string {
	return fmt.Sprintf("creator.%s.room.%s", creatorID, roomID)
}

func publishKey(req SyncRequest, eventName string) string {
	return fmt.Sprintf("%s:%s:%s:%s", req.CreatorID, req.RoomID, req.SubscriberID, eventName)
}
