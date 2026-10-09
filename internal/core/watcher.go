package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches canonical and target directories for filesystem changes with debouncing.
type FileWatcher struct {
	watcher    *fsnotify.Watcher
	m          *Manager
	debounceMs time.Duration
	notifyCh   chan struct{}
	stopCh     chan struct{}
	watchedMu  sync.Mutex
	watched    map[string]bool
}

// NewFileWatcher creates a new file watcher for canonical and target directories.
func NewFileWatcher(m *Manager, debounceDuration time.Duration) (*FileWatcher, error) {
	if debounceDuration <= 0 {
		debounceDuration = 250 * time.Millisecond
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	w := &FileWatcher{
		watcher:    fw,
		m:          m,
		debounceMs: debounceDuration,
		notifyCh:   make(chan struct{}, 1),
		stopCh:     make(chan struct{}),
		watched:    make(map[string]bool),
	}

	w.refreshWatchedPaths()
	go w.watchLoop()

	return w, nil
}

// NotifyChan returns a receive-only channel that receives a signal when a debounced change occurs.
func (w *FileWatcher) NotifyChan() <-chan struct{} {
	return w.notifyCh
}

// Close stops the watcher and frees resources.
func (w *FileWatcher) Close() error {
	close(w.stopCh)
	return w.watcher.Close()
}

// RefreshPaths updates the set of paths being watched (e.g. when targets change or skills are added).
func (w *FileWatcher) RefreshPaths() {
	w.refreshWatchedPaths()
}

func (w *FileWatcher) refreshWatchedPaths() {
	w.watchedMu.Lock()
	defer w.watchedMu.Unlock()

	pathsToWatch := make(map[string]bool)

	// Watch canonical dir and subfolders
	if info, err := os.Stat(w.m.CanonicalDir); err == nil && info.IsDir() {
		_ = filepath.Walk(w.m.CanonicalDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				pathsToWatch[path] = true
			}
			return nil
		})
	}

	// Watch target directories
	for _, target := range w.m.GetTargets() {
		targetDir := ExpandPath(target.Path)
		if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
			pathsToWatch[targetDir] = true
		}
	}

	// Add new paths
	for p := range pathsToWatch {
		if !w.watched[p] {
			if err := w.watcher.Add(p); err == nil {
				w.watched[p] = true
			}
		}
	}

	// Remove paths no longer in set
	for p := range w.watched {
		if !pathsToWatch[p] {
			_ = w.watcher.Remove(p)
			delete(w.watched, p)
		}
	}
}

func (w *FileWatcher) watchLoop() {
	var timer *time.Timer
	var timerMu sync.Mutex

	triggerDebounce := func() {
		timerMu.Lock()
		defer timerMu.Unlock()

		if timer != nil {
			timer.Stop()
		}

		timer = time.AfterFunc(w.debounceMs, func() {
			select {
			case w.notifyCh <- struct{}{}:
			default:
				// Channel already has a pending notification
			}
		})
	}

	for {
		select {
		case <-w.stopCh:
			timerMu.Lock()
			if timer != nil {
				timer.Stop()
			}
			timerMu.Unlock()
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// If a new directory was created inside canonical, add it to watcher
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					w.watchedMu.Lock()
					if !w.watched[event.Name] {
						if err := w.watcher.Add(event.Name); err == nil {
							w.watched[event.Name] = true
						}
					}
					w.watchedMu.Unlock()
				}
			}

			// Trigger debounced notification for write/create/remove/rename
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				triggerDebounce()
			}

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}
