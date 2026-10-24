package engine

import (
	"io/fs"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// startWatcher watches root recursively and debounces reindex.
func (e *Engine) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		e.log.Printf("watcher unavailable: %v", err)
		return
	}
	e.watcher = w
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				_ = w.Add(path)
			}
			return nil
		})
	}
	addTree(e.root)

	go func() {
		defer w.Close()
		var timer *time.Timer
		schedule := func() {
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(200*time.Millisecond, func() {
				e.reindex()
				// New directories may appear; refresh watches.
				addTree(e.root)
			})
		}
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) != 0 {
					schedule()
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				e.log.Printf("watcher error: %v", err)
			}
		}
	}()
}
