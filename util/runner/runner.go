package runner

// Task is a function type which returns result instance
type Task func() Result

// Do executes task and send output to channel
func Do(task Task) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		ch <- task()
	}()
	return ch
}

// Result interface wraps Value and Error methods.
type Result interface {
	Value() interface{}
	Error() error
}

// NewResult returns result instance with value as input
func NewResult(value interface{}, err error) Result {
	return result{
		value: value,
		err:   err,
	}
}

type result struct {
	value interface{}
	err   error
}

func (r result) Value() interface{} {
	return r.value
}

func (r result) Error() error {
	return r.err
}
