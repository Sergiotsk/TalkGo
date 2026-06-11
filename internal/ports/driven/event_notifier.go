package driven

// EventNotifier defines the driven port for sending asynchronous messages to connected clients.
// The signaling Hub implements this interface.
type EventNotifier interface {
	// NotifySession sends a message to the client associated with the given sessionID.
	// If the session is not connected, the message is silently dropped.
	NotifySession(sessionID string, msgType string, fields map[string]string)
}
