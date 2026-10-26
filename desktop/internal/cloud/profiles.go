package cloud

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// identifier accepts the platform's actual numeric and string IAM identifiers.
type identifier string

func (i *identifier) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*i = ""
		return nil
	}
	var text string
	if data[0] == '"' {
		if e := json.Unmarshal(data, &text); e != nil {
			return e
		}
	} else {
		var number json.Number
		if e := json.Unmarshal(data, &number); e != nil {
			return e
		}
		text = number.String()
		if strings.ContainsAny(text, ".eE+-") {
			return errors.New("invalid identifier")
		}
	}
	*i = identifier(text)
	return nil
}

type profileStore struct {
	mu   sync.Mutex
	path func() (string, error)
	ids  map[string]string
}

func NewWithProfileStore(path func() (string, error)) *Client {
	c := New()
	c.profiles.path = path
	return c
}
func (s *profileStore) ID(account, identity, accessKey string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filename := ""
	if s.path != nil {
		var e error
		filename, e = s.path()
		if e != nil {
			return "", e
		}
	}
	if s.ids == nil {
		s.ids = map[string]string{}
		if filename != "" {
			b, e := os.ReadFile(filename)
			if e != nil && !os.IsNotExist(e) {
				return "", e
			}
			if e == nil && json.Unmarshal(b, &s.ids) != nil {
				return "", errors.New("invalid profile registry")
			}
		}
	}
	digest := sha256.Sum256([]byte(account + "\x00" + identity + "\x00" + accessKey))
	key := hex.EncodeToString(digest[:])
	if id := s.ids[key]; id != "" {
		return id, nil
	}
	random := make([]byte, 16)
	if _, e := rand.Read(random); e != nil {
		return "", e
	}
	id := "credential-profile-" + hex.EncodeToString(random)
	s.ids[key] = id
	if filename != "" {
		if e := os.MkdirAll(filepath.Dir(filename), 0700); e != nil {
			return "", e
		}
		b, e := json.Marshal(s.ids)
		if e != nil {
			return "", e
		}
		tmp, e := os.CreateTemp(filepath.Dir(filename), ".profiles-")
		if e != nil {
			return "", e
		}
		defer os.Remove(tmp.Name())
		if e = tmp.Chmod(0600); e == nil {
			_, e = tmp.Write(b)
		}
		if e == nil {
			e = tmp.Sync()
		}
		ce := tmp.Close()
		if e == nil {
			e = ce
		}
		if e == nil {
			e = os.Rename(tmp.Name(), filename)
		}
		if e != nil {
			return "", e
		}
	}
	return id, nil
}
