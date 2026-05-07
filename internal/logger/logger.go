package logger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type Logger struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	enc    *json.Encoder
	old    string
	target string
}

type Event struct {
	Timestamp    string `json:"ts"`
	OldPath      string `json:"old_path"`
	TargetPath   string `json:"target_path"`
	RelativePath string `json:"relative_path"`
	Operation    string `json:"operation"`
	PID          uint32 `json:"pid,omitempty"`
	UID          uint32 `json:"uid,omitempty"`
	GID          uint32 `json:"gid,omitempty"`
	Flags        uint32 `json:"flags,omitempty"`
}

func Open(path, oldPath, targetPath string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{
		path:   path,
		file:   file,
		enc:    json.NewEncoder(file),
		old:    oldPath,
		target: targetPath,
	}, nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Logger) Path() string {
	return l.path
}

func (l *Logger) Record(ctx context.Context, operation, relativePath string, flags uint32) {
	if l == nil {
		return
	}

	event := Event{
		Timestamp:    time.Now().Format(time.RFC3339Nano),
		OldPath:      l.old,
		TargetPath:   l.target,
		RelativePath: relativePath,
		Operation:    operation,
		Flags:        flags,
	}
	if caller, ok := fuse.FromContext(ctx); ok {
		event.PID = caller.Pid
		event.UID = caller.Uid
		event.GID = caller.Gid
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.enc != nil {
		_ = l.enc.Encode(event)
	}
}
