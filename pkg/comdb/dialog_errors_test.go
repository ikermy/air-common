package comdb

import (
	"errors"
	"testing"
)

func TestDialogSentinelErrors(t *testing.T) {
	d := &DB{}

	if _, err := d.ReadDialog(0); !errors.Is(err, ErrInvalidDialogID) {
		t.Fatalf("ReadDialog(0) = %v, want ErrInvalidDialogID", err)
	}
	if err := d.DeleteDialog(1, 0); !errors.Is(err, ErrInvalidDialogID) {
		t.Fatalf("DeleteDialog(invalid dialog) = %v, want ErrInvalidDialogID", err)
	}
	if err := d.DeleteDialog(0, 1); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("DeleteDialog(invalid user) = %v, want ErrInvalidUserID", err)
	}
}
