package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher watches the canonical store and target directories.
// Notifications are debounced. Close is safe to call more than once.
type FileWatcher struct {
	watcher   *fsnotify.Watcher
	m         *Manager
	debounce  time.Duration
	notifyCh  chan struct{}
	stopCh    chan struct{}
	watchedMu sync.Mutex
	watched   map[string]bool
	once      sync.Once
	onError   func(error)
	timerMu   sync.Mutex
	timer     *time.Timer
}

// NewFileWatcher starts a debounced watcher. debounce <= 0 uses 250ms.
func NewFileWatcher(m *Manager, debounce time.Duration) (*FileWatcher, error) {
	if m == nil {
		return nil, fmt.Errorf("%w: nil manager", ErrInvalidPath)
	}
	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	w := &FileWatcher{
		watcher:  fw,
		m:        m,
		debounce: debounce,
		notifyCh: make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		watched:  map[string]bool{},
	}
	w.refreshWatchedPaths()
	go w.watchLoop()
	return w, nil
}

// NotifyChan receives one signal per debounced burst of changes.
func (w *FileWatcher) NotifyChan() <-chan struct{} { return w.notifyCh }

// SetErrorHandler receives watcher errors, including queue overflows.
func (w *FileWatcher) SetErrorHandler(fn func(error)) {
	w.watchedMu.Lock()
	w.onError = fn
	w.watchedMu.Unlock()
}

// Close stops the watcher. A second call does nothing.
func (w *FileWatcher) Close() error {
	var err error
	w.once.Do(func() {
		close(w.stopCh)
		w.timerMu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.timerMu.Unlock()
		err = w.watcher.Close()
	})
	return err
}

// RefreshPaths updates the watched set after targets or skills change.
func (w *FileWatcher) RefreshPaths() { w.refreshWatchedPaths() }

func (w *FileWatcher) refreshWatchedPaths() {
	w.watchedMu.Lock()
	defer w.watchedMu.Unlock()
	want := map[string]bool{}
	if info, err := os.Lstat(w.m.CanonicalDir); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		_ = filepath.WalkDir(w.m.CanonicalDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() || d.Type()&os.ModeSymlink != 0 {
				if d != nil && d.IsDir() && d.Type()&os.ModeSymlink != 0 {
					return filepath.SkipDir
				}
				return nil
			}
			want[path] = true
			return nil
		})
	}
	for _, target := range w.m.GetTargets() {
		dir, err := ExpandPath(target.Path)
		if err != nil {
			continue
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		want[dir] = true
	}
	for p := range want {
		if !w.watched[p] {
			if err := w.watcher.Add(p); err == nil {
				w.watched[p] = true
			}
		}
	}
	for p := range w.watched {
		if !want[p] {
			_ = w.watcher.Remove(p)
			delete(w.watched, p)
		}
	}
}

func (w *FileWatcher) watchLoop() {
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Lstat(event.Name); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
					w.watchedMu.Lock()
					if !w.watched[event.Name] {
						if err := w.watcher.Add(event.Name); err == nil {
							w.watched[event.Name] = true
						}
					}
					w.watchedMu.Unlock()
				}
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				w.arm()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			if err == nil {
				continue
			}
			w.watchedMu.Lock()
			fn := w.onError
			w.watchedMu.Unlock()
			if fn != nil {
				fn(err)
			}
		}
	}
}

func (w *FileWatcher) arm() {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounce, func() {
		select {
		case <-w.stopCh:
		case w.notifyCh <- struct{}{}:
		default:
		}
	})
}
