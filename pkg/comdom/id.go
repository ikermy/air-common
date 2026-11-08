package comdom

import (
	"errors"
	"strconv"
)

// ErrZeroID is returned when a required domain identifier is zero.
var ErrZeroID = errors.New("comdom: zero id")

// Domain identifiers are defined types over uint64. Using defined types (not
// aliases) prevents accidental mixing of UserID and DialogID in function
// signatures while keeping the wire/JSON representation numeric.
type (
	UserID      uint64
	DialogID    uint64
	ResponderID uint64
	MessageID   uint64
	BotID       uint64
	ChannelID   uint64
)

func UserIDFromUint64(v uint64) UserID           { return UserID(v) }
func DialogIDFromUint64(v uint64) DialogID       { return DialogID(v) }
func ResponderIDFromUint64(v uint64) ResponderID { return ResponderID(v) }
func MessageIDFromUint64(v uint64) MessageID     { return MessageID(v) }
func BotIDFromUint64(v uint64) BotID             { return BotID(v) }
func ChannelIDFromUint64(v uint64) ChannelID     { return ChannelID(v) }

func (id UserID) Uint64() uint64      { return uint64(id) }
func (id DialogID) Uint64() uint64    { return uint64(id) }
func (id ResponderID) Uint64() uint64 { return uint64(id) }
func (id MessageID) Uint64() uint64   { return uint64(id) }
func (id BotID) Uint64() uint64       { return uint64(id) }
func (id ChannelID) Uint64() uint64   { return uint64(id) }

func (id UserID) IsZero() bool      { return id == 0 }
func (id DialogID) IsZero() bool    { return id == 0 }
func (id ResponderID) IsZero() bool { return id == 0 }
func (id MessageID) IsZero() bool   { return id == 0 }
func (id BotID) IsZero() bool       { return id == 0 }
func (id ChannelID) IsZero() bool   { return id == 0 }

func (id UserID) Validate() error      { return validateID(id == 0) }
func (id DialogID) Validate() error    { return validateID(id == 0) }
func (id ResponderID) Validate() error { return validateID(id == 0) }
func (id MessageID) Validate() error   { return validateID(id == 0) }
func (id BotID) Validate() error       { return validateID(id == 0) }
func (id ChannelID) Validate() error   { return validateID(id == 0) }

func validateID(zero bool) error {
	if zero {
		return ErrZeroID
	}
	return nil
}

func (id UserID) String() string      { return strconv.FormatUint(uint64(id), 10) }
func (id DialogID) String() string    { return strconv.FormatUint(uint64(id), 10) }
func (id ResponderID) String() string { return strconv.FormatUint(uint64(id), 10) }
func (id MessageID) String() string   { return strconv.FormatUint(uint64(id), 10) }
func (id BotID) String() string       { return strconv.FormatUint(uint64(id), 10) }
func (id ChannelID) String() string   { return strconv.FormatUint(uint64(id), 10) }
