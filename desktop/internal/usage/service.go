// Package usage adapts existing Runtime resources for the desktop.
package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type Service struct {
	conn            *connection.Client
	cloud           *cloud.Client
	target          func() deployment.Target
	skillUploads    *skillUploadState
	putSkillArchive func(context.Context, string, string, []byte) (cloud.SkillArchiveObject, *dto.SafeError)
}

func New(conn *connection.Client, platform *cloud.Client, target ...func() deployment.Target) *Service {
	s := &Service{conn: conn, cloud: platform, skillUploads: &skillUploadState{operations: map[string]*skillUploadOperation{}}}
	if platform != nil {
		s.putSkillArchive = platform.PutSkillArchive
	}
	if len(target) > 0 {
		s.target = target[0]
	}
	return s
}

func (s *Service) request(ctx context.Context, epoch uint64, method, path string, body, output any) *dto.SafeError {
	scope := s.conn.State().Scope
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return dto.Failure("invalidInput", "desktop", "never", 0)
		}
	}
	resp, err := s.conn.Request(ctx, epoch, method, path, data, nil)
	if err != nil {
		if method != "GET" && err.HTTPStatus >= 500 {
			return dto.Failure("outcomeUnknown", err.Plane, "verifyOnly", err.HTTPStatus)
		}
		return err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if !s.conn.Current(epoch) || s.conn.State().Scope != scope {
		return dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if readErr != nil || len(raw) > 4<<20 || !json.Valid(raw) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, output) != nil {
		code, retry := "invalidResponse", "readOnly"
		if method != "GET" {
			code, retry = "outcomeUnknown", "verifyOnly"
		}
		return dto.Failure(code, "runtimeIngress", retry, resp.StatusCode)
	}
	return nil
}
