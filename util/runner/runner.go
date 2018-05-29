package runner

type Task func() Result

func Do(task Task) <-chan Result {
	ch := make(chan Result, 1)
	go func() {
		ch <- task()
	}()
	return ch
}

type Result interface {
	Value() interface{}
	Error() error
}

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
