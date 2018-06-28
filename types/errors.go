package types

func (e ErrInvalidPayload) Error() string {
	return e.Message
}

func (e ErrInternalError) Error() string {
	return e.Message
}

func (e ErrResourceNotFound) Error() string {
	return e.Message
}
