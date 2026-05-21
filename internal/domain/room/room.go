package room

import (
	"errors"
	"time"
)

var (
	ErrInvalidLanguageCode = errors.New("invalid language code: must be ISO 639-1 (2 characters)")
)

// Room represents an active translation room between two languages.
type Room struct {
	ID         string
	SourceLang string
	TargetLang string
	CreatedAt  time.Time
	Active     bool
}

// NewRoom creates and initializes a new Room with language validation.
func NewRoom(id, sourceLang, targetLang string) (*Room, error) {
	if len(sourceLang) != 2 || len(targetLang) != 2 {
		return nil, ErrInvalidLanguageCode
	}
	return &Room{
		ID:         id,
		SourceLang: sourceLang,
		TargetLang: targetLang,
		CreatedAt:  time.Now(),
		Active:     true,
	}, nil
}
