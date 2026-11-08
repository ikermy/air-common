package comdb

import "errors"

// Sentinel errors returned by comdb. Callers must use errors.Is instead of
// matching error strings.
var (
	ErrInvalidDialogID  = errors.New("invalid dialog id")
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrUserNotFound     = errors.New("user not found")
	ErrDemoUserDeletion = errors.New("demo user cannot delete dialogs")
	ErrDialogNotFound   = errors.New("dialog not found or not owned by user")
)
