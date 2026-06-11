package roomsvc

import "errors"

// ErrMissingLang is returned when a join request does not include a language code.
var ErrMissingLang = errors.New("lang is required")

// ErrLangNotSupported is returned when the participant's language does not match
// either the room's SourceLang or TargetLang.
var ErrLangNotSupported = errors.New("lang not supported in room")

// ErrNilDependency is returned when NewService receives a nil driven port.
var ErrNilDependency = errors.New("nil dependency")

// ErrShortCodeConflict is returned internally when a generated short code already exists.
var ErrShortCodeConflict = errors.New("short code already in use")
