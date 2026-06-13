package models

import "time"

type Message struct {
	ID        int64
	ChatID    int64
	senderID  int64
	text      string
	CreatedAt time.Time
}
