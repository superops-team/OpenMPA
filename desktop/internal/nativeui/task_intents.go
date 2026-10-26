package nativeui

import (
	"encoding/json"
	"errors"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// taskIntents retains exact request tokens across page changes and app restarts.
// No restart retries a mutation; recovery is an explicit user action.
type taskIntent struct {
	Assistant *assistantIntent        `json:"assistant,omitempty"`
	Import    *importIntent           `json:"import,omitempty"`
	Create    *schedule.CreateRequest `json:"create,omitempty"`
	Run       *schedule.RunRequest    `json:"run,omitempty"`
}
type taskIntents struct {
	mu     sync.Mutex
	path   func() (string, error)
	memory map[string]taskIntent
}

func (s *taskIntents) read() (map[string]taskIntent, string, error) {
	if s.path == nil {
		if s.memory == nil {
			s.memory = map[string]taskIntent{}
		}
		b, _ := json.Marshal(s.memory)
		var copy map[string]taskIntent
		_ = json.Unmarshal(b, &copy)
		return copy, "", nil
	}
	path, e := s.path()
	if e != nil || !filepath.IsAbs(path) {
		return nil, "", errors.New("intent path unavailable")
	}
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return map[string]taskIntent{}, path, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, "", errors.New("invalid intent store")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, "", e
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 8<<20))
	decoder.DisallowUnknownFields()
	var data map[string]taskIntent
	if e = decoder.Decode(&data); e != nil || decoder.Decode(new(any)) != io.EOF || data == nil {
		return nil, "", errors.New("invalid intent store")
	}
	return data, path, nil
}
func (s *taskIntents) load(scope string) (taskIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _, e := s.read()
	if e != nil {
		return taskIntent{}, e
	}
	v := data[scope]
	b, _ := json.Marshal(v)
	var out taskIntent
	_ = json.Unmarshal(b, &out)
	return out, nil
}
func (s *taskIntents) change(scope string, kind string, create *schedule.CreateRequest, run *schedule.RunRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scope == "" {
		return errors.New("missing scope")
	}
	data, path, e := s.read()
	if e != nil {
		return e
	}
	v := data[scope]
	if kind == "create" {
		if create != nil && v.Create != nil && v.Create.ClientToken != create.ClientToken {
			return errors.New("pending create exists")
		}
		if create != nil && v.Create != nil {
			old, next := *v.Create, *create
			old.ConnectionEpoch = 0
			next.ConnectionEpoch = 0
			a, _ := json.Marshal(old)
			b, _ := json.Marshal(next)
			if string(a) != string(b) {
				return errors.New("pending create changed")
			}
		}
		v.Create = create
	} else {
		if run != nil && v.Run != nil && v.Run.ClientToken != run.ClientToken {
			return errors.New("pending run exists")
		}
		if run != nil && v.Run != nil {
			old, next := *v.Run, *run
			old.ConnectionEpoch = 0
			next.ConnectionEpoch = 0
			a, _ := json.Marshal(old)
			b, _ := json.Marshal(next)
			if string(a) != string(b) {
				return errors.New("pending run changed")
			}
		}
		v.Run = run
	}
	if v.Create == nil && v.Run == nil && v.Import == nil && v.Assistant == nil {
		delete(data, scope)
	} else {
		data[scope] = v
	}
	return s.write(data, path)
}

func (s *taskIntents) saveImport(scope string, input *importIntent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, path, e := s.read()
	if e != nil {
		return e
	}
	v := data[scope]
	if input != nil && v.Import != nil {
		old, _ := json.Marshal(v.Import)
		next, _ := json.Marshal(input)
		var oldInput, newInput importIntent
		_ = json.Unmarshal(old, &oldInput)
		_ = json.Unmarshal(next, &newInput)
		if oldInput.Text != nil && newInput.Text != nil {
			oldInput.Text.ConnectionEpoch = 0
			newInput.Text.ConnectionEpoch = 0
		}
		if oldInput.File != nil && newInput.File != nil {
			oldInput.File.ConnectionEpoch = 0
			newInput.File.ConnectionEpoch = 0
		}
		old, _ = json.Marshal(oldInput)
		next, _ = json.Marshal(newInput)
		if string(old) != string(next) {
			return errors.New("pending import exists")
		}
	}
	v.Import = input
	if v.Create == nil && v.Run == nil && v.Import == nil && v.Assistant == nil {
		delete(data, scope)
	} else {
		data[scope] = v
	}
	return s.write(data, path)
}
func (s *taskIntents) write(data map[string]taskIntent, path string) error {
	b, e := json.Marshal(data)
	if e != nil || len(b) > 8<<20 {
		return errors.New("intent store too large")
	}
	if path == "" {
		s.memory = data
		return nil
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".native-task-intents-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeError := f.Close()
	if e == nil {
		e = closeError
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	return e
}

func (s *taskIntents) saveAssistant(scope string, input *assistantIntent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, path, e := s.read()
	if e != nil {
		return e
	}
	v := data[scope]
	if input != nil && v.Assistant != nil && *input != *v.Assistant {
		return errors.New("pending assistant exists")
	}
	v.Assistant = input
	if v.Create == nil && v.Run == nil && v.Import == nil && v.Assistant == nil {
		delete(data, scope)
	} else {
		data[scope] = v
	}
	return s.write(data, path)
}
