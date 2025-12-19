package watcher

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileChange represents a file change event
type FileChange struct {
	Path string
}

// Watcher manages file watching for auto-publish
type Watcher struct {
	watcher    *fsnotify.Watcher
	files      map[string]bool
	onChange   func(path string)
	debounce   map[string]time.Time
	debounceMu sync.Mutex
	debounceMs time.Duration
	stopChan   chan struct{}
	mu         sync.Mutex
}

// New creates a new file watcher
func New(onChange func(path string)) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher:    fsWatcher,
		files:      make(map[string]bool),
		onChange:   onChange,
		debounce:   make(map[string]time.Time),
		debounceMs: 300 * time.Millisecond,
		stopChan:   make(chan struct{}),
	}

	go w.run()
	return w, nil
}

// run processes file system events
func (w *Watcher) run() {
	for {
		select {
		case <-w.stopChan:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				w.handleChange(event.Name)
			}
		case <-w.watcher.Errors:
			// Log error but continue
		}
	}
}

// handleChange processes a file change with debouncing
func (w *Watcher) handleChange(path string) {
	w.debounceMu.Lock()
	lastChange, exists := w.debounce[path]
	now := time.Now()

	if exists && now.Sub(lastChange) < w.debounceMs {
		w.debounceMu.Unlock()
		return
	}

	w.debounce[path] = now
	w.debounceMu.Unlock()

	if w.onChange != nil {
		w.onChange(path)
	}
}

// AddFile starts watching a file
func (w *Watcher) AddFile(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.files[path] {
		return nil
	}

	if err := w.watcher.Add(path); err != nil {
		return err
	}

	w.files[path] = true
	return nil
}

// RemoveFile stops watching a file
func (w *Watcher) RemoveFile(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.files[path] {
		return nil
	}

	if err := w.watcher.Remove(path); err != nil {
		return err
	}

	delete(w.files, path)
	return nil
}

// Clear removes all watched files
func (w *Watcher) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for path := range w.files {
		w.watcher.Remove(path)
	}
	w.files = make(map[string]bool)
}

// Close stops the watcher
func (w *Watcher) Close() error {
	close(w.stopChan)
	return w.watcher.Close()
}
