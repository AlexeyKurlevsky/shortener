package audit

import (
	"context"
	"encoding/json"
	"os"
	"sync"
)

type FileObserver struct {
	mu   sync.Mutex
	file *os.File
}

func NewFileObserver(path string) (*FileObserver, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileObserver{file: f}, nil
}

func (o *FileObserver) Notify(_ context.Context, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	o.mu.Lock()
	defer o.mu.Unlock()
	_, err = o.file.Write(data)
	return err
}

func (o *FileObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.file == nil {
		return nil
	}
	err := o.file.Close()
	o.file = nil
	return err
}
