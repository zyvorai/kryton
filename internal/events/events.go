// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/kryton/internal/id"
)

type Event struct {
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Source          string         `json:"source"`
	Type            string         `json:"type"`
	Subject         string         `json:"subject,omitempty"`
	Time            time.Time      `json:"time"`
	DataContentType string         `json:"datacontenttype"`
	Data            map[string]any `json:"data,omitempty"`
}

type Bus struct {
	mu            sync.RWMutex
	history       []Event
	maxHistory    int
	subs          map[chan Event]struct{}
	webhook       string
	webhookSecret string
	store         *fileStore
	http          *http.Client
	log           *slog.Logger
}

func New(maxHistory int, webhook, webhookSecret, filePath string, log *slog.Logger) (*Bus, error) {
	if maxHistory < 1 {
		maxHistory = 500
	}
	store, err := openFileStore(filePath)
	if err != nil {
		return nil, err
	}
	b := &Bus{
		maxHistory: maxHistory, subs: map[chan Event]struct{}{},
		webhook: webhook, webhookSecret: webhookSecret, store: store,
		http: &http.Client{Timeout: 3 * time.Second}, log: log,
	}
	if store != nil {
		loaded, err := loadFileStore(filePath, maxHistory)
		if err != nil {
			return nil, err
		}
		b.history = loaded
	}
	return b, nil
}

func (b *Bus) Publish(ctx context.Context, eventType, subject string, data map[string]any) Event {
	e := Event{SpecVersion: "1.0", ID: id.New(), Source: "kryton://control-plane", Type: eventType, Subject: subject, Time: time.Now().UTC(), DataContentType: "application/json", Data: data}
	b.mu.Lock()
	b.history = append([]Event{e}, b.history...)
	if len(b.history) > b.maxHistory {
		b.history = b.history[:b.maxHistory]
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
	b.mu.Unlock()
	if b.store != nil {
		if err := b.store.append(e); err != nil && b.log != nil {
			b.log.Warn("event persist failed", "error", err)
		}
	}
	if b.webhook != "" {
		b.deliverWebhook(ctx, e)
	}
	return e
}

func (b *Bus) SetWebhookURL(url string) {
	b.mu.Lock()
	b.webhook = strings.TrimSpace(url)
	b.mu.Unlock()
}

func (b *Bus) List(limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit < 1 || limit > len(b.history) {
		limit = len(b.history)
	}
	out := make([]Event, limit)
	copy(out, b.history[:limit])
	return out
}

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

func (b *Bus) deliverWebhook(parent context.Context, e Event) {
	body, err := json.Marshal(e)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.webhook, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")
	if b.webhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(b.webhookSecret))
		_, _ = mac.Write(body)
		req.Header.Set("X-Kryton-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	res, err := b.http.Do(req)
	if err != nil {
		b.log.Warn("event webhook failed", "error", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b.log.Warn("event webhook returned error", "status", res.StatusCode)
	}
}
