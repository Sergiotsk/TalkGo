package mocks

import (
	"sync"

	"github.com/Sergiotsk/TalkGo/internal/ports/driven"
)

var _ driven.EventNotifier = (*MockEventNotifier)(nil)

// Notification records a single event sent via MockEventNotifier.
type Notification struct {
	SessionID string
	MsgType   string
	Fields    map[string]string
}

// MockEventNotifier is a test double for driven.EventNotifier.
// All calls are recorded in Notifications and are safe for concurrent use.
type MockEventNotifier struct {
	mu            sync.Mutex
	Notifications []Notification
}

// NotifySession implements driven.EventNotifier.
func (m *MockEventNotifier) NotifySession(sessionID, msgType string, fields map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Notifications = append(m.Notifications, Notification{
		SessionID: sessionID,
		MsgType:   msgType,
		Fields:    fields,
	})
}

// NotificationsFor returns all notifications sent to the given sessionID.
func (m *MockEventNotifier) NotificationsFor(sessionID string) []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Notification
	for _, n := range m.Notifications {
		if n.SessionID == sessionID {
			result = append(result, n)
		}
	}
	return result
}
