package watcher

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type EventSource interface {
	Events() <-chan string
	Errors() <-chan error
	Close() error
}

type FSNotifySource struct {
	watcher *fsnotify.Watcher
	events  chan string
}

func NewFSNotifySource(dir string) (*FSNotifySource, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	source := &FSNotifySource{watcher: w, events: make(chan string, 16)}
	go source.forward()
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("watch %q: %w", dir, err)
	}
	return source, nil
}

func (s *FSNotifySource) Events() <-chan string { return s.events }

func (s *FSNotifySource) Errors() <-chan error { return s.watcher.Errors }

func (s *FSNotifySource) Close() error {
	return s.watcher.Close()
}

func (s *FSNotifySource) forward() {
	defer close(s.events)
	for event := range s.watcher.Events {
		if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Chmod) {
			s.events <- event.Name
		}
	}
}

type Loop struct {
	Service      Service
	Source       EventSource
	ConfigPath   string
	Debounce     time.Duration
	ErrorHandler func(error)
}

func (l Loop) Run(ctx context.Context) error {
	if err := l.Service.Sync(ctx); err != nil {
		return err
	}
	debounce := l.Debounce
	if debounce <= 0 {
		debounce = 500 * time.Millisecond
	}
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	pending := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case path, ok := <-l.Source.Events():
			if !ok {
				return nil
			}
			if filepath.Base(path) != filepath.Base(l.ConfigPath) {
				continue
			}
			pending = true
			timer.Reset(debounce)
		case err, ok := <-l.Source.Errors():
			if !ok {
				return nil
			}
			l.handle(err)
		case <-timer.C:
			if pending {
				pending = false
				if err := l.Service.Sync(ctx); err != nil {
					l.handle(err)
				}
			}
		}
	}
}

func (l Loop) handle(err error) {
	if err == nil || l.ErrorHandler == nil {
		return
	}
	l.ErrorHandler(err)
}
