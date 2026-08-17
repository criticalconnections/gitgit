package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// fireWebhooks delivers an event payload to every matching active webhook.
// Delivery is asynchronous and best-effort.
func fireWebhooks(repo *Repo, event string, payload map[string]any) {
	hooks := listWebhooks(repo.ID)
	if len(hooks) == 0 {
		return
	}
	payload["event"] = event
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	for _, h := range hooks {
		if !h.Active || !h.HasEvent(event) {
			continue
		}
		go deliverWebhook(h, event, body)
	}
}

func deliverWebhook(h *Webhook, event string, body []byte) {
	req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitGit-Event", event)
	req.Header.Set("User-Agent", "gitgit-webhook")
	if h.Secret != "" {
		mac := hmac.New(sha256.New, []byte(h.Secret))
		mac.Write(body)
		req.Header.Set("X-GitGit-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook %s (%s): %v", h.URL, event, err)
		return
	}
	resp.Body.Close()
	log.Printf("webhook %s (%s): %s", h.URL, event, resp.Status)
}
