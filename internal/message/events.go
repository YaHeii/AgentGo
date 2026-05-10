package message

import (
	"fmt"
)

type MessageStatus int

const (
	StatusPending   MessageStatus = iota // sending
	StatusSent                           // successful sent
	StatusStreaming                      // streaming update
	StatusFailed                         // fail sent
)

func (s MessageStatus) String() string {
	switch s {
	case StatusPending:
		return "PENDING"
	case StatusSent:
		return "SENT"
	case StatusStreaming:
		return "STREAMING"
	case StatusFailed:
		return "FAILED"
	default:
		return fmt.Sprintf("UNKNOWN_MSG_STATUS(%d)", s)
	}
}

type MessageEvent struct {
	Status  MessageStatus
	Message *Message 
}