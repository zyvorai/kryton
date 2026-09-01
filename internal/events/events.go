package events

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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
	mu         sync.RWMutex
	history    []Event
	maxHistory int
	subs       map[chan Event]struct{}
	webhook    string
	http       *http.Client
	log        *slog.Logger
}

func New(maxHistory int, webhook string, log *slog.Logger) *Bus {
	if maxHistory < 1 {
		maxHistory = 500
	}
	return &Bus{maxHistory: maxHistory, subs: map[chan Event]struct{}{}, webhook: webhook, http: &http.Client{Timeout: 3 * time.Second}, log: log}
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
	if b.webhook != "" {
		b.deliverWebhook(ctx, e)
	}
	return e
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
