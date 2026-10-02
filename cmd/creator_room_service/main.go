package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/creator-chat-adr-go/creatorroom"
)

func main() {
	listen := flag.String("listen", ":8080", "http listen address")
	flag.Parse()

	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := creatorroom.NewInfraiClient(key, http.DefaultClient)
	service := creatorroom.NewService(client)

	mux := http.NewServeMux()
	mux.HandleFunc("/creator-room/sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req creatorroom.SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		resp, err := service.SyncPurchase(ctx, req)
		if err != nil {
			writeDomainError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("/creator-room/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req creatorroom.TokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		resp, err := service.IssueSubscriberToken(ctx, req)
		if err != nil {
			writeDomainError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	})

	log.Printf("listening on %s", *listen)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	var inputErr creatorroom.InputError
	var apiErr creatorroom.InfraiError

	switch {
	case errors.As(err, &inputErr):
		http.Error(w, inputErr.Error(), http.StatusBadRequest)
	case errors.As(err, &apiErr):
		status := http.StatusBadGateway
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			status = http.StatusBadRequest
		}
		http.Error(w, apiErr.Error(), status)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
