package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

type spinner struct {
	w       io.Writer
	total   int
	mu      sync.Mutex
	done    int
	stopCh  chan struct{}
	stopped chan struct{}
}

func newSpinner(w io.Writer, total int) *spinner {
	return &spinner{
		w:       w,
		total:   total,
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (s *spinner) start() {
	go func() {
		defer close(s.stopped)
		i := 0
		s.render(spinnerFrames[0])
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				i++
				s.render(spinnerFrames[i%len(spinnerFrames)])
			}
		}
	}()
}

func (s *spinner) render(frame rune) {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	fmt.Fprintf(s.w, "\r%c [%d/%d] Testing mutants...", frame, done, s.total)
}

func (s *spinner) update(done int) {
	s.mu.Lock()
	s.done = done
	s.mu.Unlock()
}

func (s *spinner) stop() {
	close(s.stopCh)
	<-s.stopped
	fmt.Fprintf(s.w, "\r\033[K")
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
