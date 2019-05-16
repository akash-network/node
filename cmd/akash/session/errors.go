package session

import (
	"fmt"
)

type TooManyKeysForDefaultError struct{ KeysCount int }

func (e *TooManyKeysForDefaultError) Error() string {
	return fmt.Sprintf(tooManyKeysErrMsg, e.KeysCount)
}

type NoKeysForDefaultError struct{}

func (e NoKeysForDefaultError) Error() string {
	return ("no keys found locally, need at least one key")
}

var (
	tooManyKeysErrMsg = "Unable to select a default key.\nToo many keys are stored locally to pick a default, a key is selected as the default only when there is a single key present. Found %d keys instead of 1."
)
