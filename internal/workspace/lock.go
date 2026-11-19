package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Active describes the process holding the installation lock.
type Active struct {
	Workspace string    `json:"workspace"`
	Root      string    `json:"root,omitempty"`
	Addr      string    `json:"addr"`
	PID       int       `json:"pid"`
	Started   time.Time `json:"started"`
}

// Lock is the per-user installation lock. Only one Athena workspace runs at a
// time; the OS releases the lock when the process exits, even after a crash.
type Lock struct {
	file *os.File
}

// LockPath is where the installation lock lives for this user.
func LockPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "athena", "active.lock"), nil
}

// Acquire takes the installation lock or reports the workspace already running.
func Acquire(info Active) (*Lock, error) {
	path, err := LockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			var active Active
			if data, readErr := os.ReadFile(path); readErr == nil && json.Unmarshal(data, &active) == nil && active.Workspace != "" {
				return nil, fmt.Errorf("Athena is already running %s at http://%s (pid %d); stop it first", active.Workspace, active.Addr, active.PID)
			}
			return nil, errors.New("another Athena process is already running; stop it first")
		}
		return nil, err
	}
	info.PID = os.Getpid()
	info.Started = time.Now().UTC()
	data, err := json.Marshal(info)
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.WriteAt(data, 0); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Release clears the lock record and releases the lock.
func (l *Lock) Release() {
	_ = l.file.Truncate(0)
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
