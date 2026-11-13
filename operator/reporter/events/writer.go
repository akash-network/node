// Package events writes the reporter's JSON-lines events to stdout in the TS reporter's schema.
package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sync"
)

const queueSize = 1024

// Writer serializes events as one JSON line each through a single goroutine.
type Writer struct {
	mu     sync.RWMutex
	closed bool
	lines  chan []byte
	done   chan struct{}
	err    error
}

// NewWriter starts a Writer that writes lines to out.
func NewWriter(out io.Writer) *Writer {
	w := &Writer{
		lines: make(chan []byte, queueSize),
		done:  make(chan struct{}),
	}

	go w.run(bufio.NewWriter(out))

	return w
}

// Emit queues v as one JSON line. It is safe for concurrent use; events emitted after Close are dropped.
func (w *Writer) Emit(v any) {
	line := encode(v)

	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return
	}

	w.lines <- line
}

// Close drains the queue, flushes the output and returns the first write error.
func (w *Writer) Close() error {
	w.mu.Lock()

	if !w.closed {
		w.closed = true
		close(w.lines)
	}

	w.mu.Unlock()

	<-w.done

	return w.err
}

func (w *Writer) run(out *bufio.Writer) {
	defer close(w.done)

	for line := range w.lines {
		if w.err == nil {
			_, w.err = out.Write(line)
		}

		if len(w.lines) == 0 && w.err == nil {
			w.err = out.Flush()
		}
	}

	if w.err == nil {
		w.err = out.Flush()
	}
}

// encode renders v like the TS reporter's JSON.stringify: compact, with <, > and & left unescaped.
func encode(v any) []byte {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		buf.Reset()
		buf.WriteString(`{"error":"event encoding failed"}` + "\n")
	}

	return buf.Bytes()
}
