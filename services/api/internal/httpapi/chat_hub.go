package httpapi

import (
	"encoding/json"
	"sync"
)

type chatEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type chatHub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan chatEvent]struct{}
}

func newChatHub() *chatHub {
	return &chatHub{subscribers: make(map[string]map[chan chatEvent]struct{})}
}

func (hub *chatHub) subscribe(conversationID string) (<-chan chatEvent, func()) {
	channel := make(chan chatEvent, 8)
	hub.mu.Lock()
	if hub.subscribers[conversationID] == nil {
		hub.subscribers[conversationID] = make(map[chan chatEvent]struct{})
	}
	hub.subscribers[conversationID][channel] = struct{}{}
	hub.mu.Unlock()
	return channel, func() {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		if subscribers := hub.subscribers[conversationID]; subscribers != nil {
			delete(subscribers, channel)
			if len(subscribers) == 0 {
				delete(hub.subscribers, conversationID)
			}
		}
		close(channel)
	}
}

func (hub *chatHub) publish(conversationID, eventType string, payload interface{}) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for channel := range hub.subscribers[conversationID] {
		select {
		case channel <- chatEvent{Type: eventType, Payload: payload}:
		default:
		}
	}
}

func encodeChatEvent(event chatEvent) ([]byte, error) { return json.Marshal(event) }
