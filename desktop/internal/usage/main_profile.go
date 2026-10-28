package usage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type MainProfileStatus string

const (
	MainProfileConfigured                MainProfileStatus = "configured"
	MainProfileUnsupported               MainProfileStatus = "unsupported"
	MainProfileUnconfigured              MainProfileStatus = "unconfigured"
	MainProfileUnsupportedOrUnconfigured MainProfileStatus = "unsupportedOrUnconfigured"
	MainProfileUnavailable               MainProfileStatus = "unavailable"
)

type MainProfileModel struct {
	ID string `json:"id"`
}

type MainProfileSkill struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Content string `json:"content"`
}

// MainProfile affects only Main. Shared AGENTS.md can also affect Worker.
type MainProfile struct {
	AgentsMD string             `json:"agentsMd"`
	Model    *MainProfileModel  `json:"model"`
	Skills   []MainProfileSkill `json:"skills"`
}

type MainProfileRecord struct {
	MPAInstanceID       string      `json:"mpaInstanceId"`
	Revision            int64       `json:"revision"`
	Digest              string      `json:"digest"`
	ETag                string      `json:"etag"`
	Profile             MainProfile `json:"profile"`
	SourceProfileID     *string     `json:"sourceProfileId"`
	SourceProfileDigest *string     `json:"sourceProfileDigest"`
	UpdatedBy           string      `json:"updatedBy"`
	CreatedAt           string      `json:"createdAt"`
}

type MainProfileState struct {
	Status          MainProfileStatus  `json:"status"`
	Record          *MainProfileRecord `json:"record,omitempty"`
	ConnectionEpoch uint64             `json:"connectionEpoch"`
	snapshot        *mainProfileSnapshot
}

type mainProfileSnapshot struct {
	owner     *Service
	epoch     uint64
	scope     string
	record    MainProfileRecord
	attempted atomic.Bool
}

var mainProfileSkillName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ReadMainProfile requires the MPA instance identity supplied by Runtime/session
// metadata. The platform's r- Runtime ID is not an interchangeable identifier.
func (s *Service) ReadMainProfile(ctx context.Context, epoch uint64, mpaInstanceID string) (MainProfileState, *dto.SafeError) {
	scope := s.conn.State().Scope
	if !resourceIdentifierValid(mpaInstanceID) || strings.HasPrefix(strings.ToLower(mpaInstanceID), "r-") {
		return MainProfileState{}, resourceInputError()
	}
	record, err := s.mainProfileRequest(ctx, epoch, http.MethodGet, mpaInstanceID, nil, nil)
	if err != nil {
		return MainProfileState{Status: mainProfileFailureStatus(err), ConnectionEpoch: epoch}, err
	}
	state := s.mainProfileState(epoch, scope, record)
	if s.conn.State().Scope != scope {
		return MainProfileState{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return state, nil
}

// SaveMainProfileInstructions can only update a profile read by this Service at
// this epoch. Every attempted write consumes its snapshot; recovery is a read,
// never automatic creation or replay after a conflict or uncertain outcome.
func (s *Service) SaveMainProfileInstructions(ctx context.Context, epoch uint64, current MainProfileState, instructions string) (MainProfileState, *dto.SafeError) {
	snapshot := current.snapshot
	if current.Status != MainProfileConfigured || snapshot == nil || snapshot.owner != s {
		return MainProfileState{}, resourceInputError()
	}
	if snapshot.epoch != epoch || snapshot.scope != s.conn.State().Scope {
		return MainProfileState{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return MainProfileState{}, err
	}
	profile := cloneMainProfileRecord(snapshot.record).Profile
	profile.AgentsMD = instructions
	if !mainProfileValid(profile) || snapshot.record.Revision >= 9999999999 {
		return MainProfileState{}, resourceInputError()
	}
	body := struct {
		Profile             MainProfile `json:"profile"`
		SourceProfileID     *string     `json:"sourceProfileId"`
		SourceProfileDigest *string     `json:"sourceProfileDigest"`
	}{profile, snapshot.record.SourceProfileID, snapshot.record.SourceProfileDigest}
	data, marshalErr := json.Marshal(body)
	if marshalErr != nil {
		return MainProfileState{}, resourceInputError()
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return MainProfileState{}, dto.Failure("unavailable", "desktop", "never", 0)
	}
	if !snapshot.attempted.CompareAndSwap(false, true) {
		return MainProfileState{}, dto.Failure("conflict", "desktop", "reload", 0)
	}
	headers := http.Header{"If-Match": {snapshot.record.ETag}, "Idempotency-Key": {hex.EncodeToString(key)}}
	record, err := s.mainProfileRequest(ctx, epoch, http.MethodPut, snapshot.record.MPAInstanceID, data, headers)
	if err != nil {
		return MainProfileState{Status: mainProfileFailureStatus(err), ConnectionEpoch: epoch}, err
	}
	expectedRevision := snapshot.record.Revision
	if !reflect.DeepEqual(profile, snapshot.record.Profile) {
		expectedRevision++
	}
	if record.Revision != expectedRevision || !reflect.DeepEqual(record.Profile, profile) ||
		!reflect.DeepEqual(record.SourceProfileID, body.SourceProfileID) ||
		!reflect.DeepEqual(record.SourceProfileDigest, body.SourceProfileDigest) {
		return MainProfileState{}, resourceResponseError(true)
	}
	if s.conn.State().Scope != snapshot.scope {
		return MainProfileState{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	return s.mainProfileState(epoch, snapshot.scope, record), nil
}

func (s *Service) mainProfileRequest(ctx context.Context, epoch uint64, method, instanceID string, body []byte, headers http.Header) (MainProfileRecord, *dto.SafeError) {
	scope := s.conn.State().Scope
	path := "/api/v1/agents/" + url.PathEscape(instanceID) + "/main-profile"
	response, err := s.conn.Request(ctx, epoch, method, path, body, headers)
	if err != nil {
		// Unlike an ordinary missing endpoint, 404 here can mean an existing
		// capability has no profile. Keep the ambiguity and never initialize it.
		if method != http.MethodGet && (err.Code == "invalidResponse" || err.HTTPStatus >= 500) {
			err = dto.Failure("outcomeUnknown", err.Plane, "verifyOnly", err.HTTPStatus)
		}
		return MainProfileRecord{}, err
	}
	defer response.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err := s.resourceEpochError(epoch); err != nil {
		return MainProfileRecord{}, err
	}
	if s.conn.State().Scope != scope {
		return MainProfileRecord{}, dto.Failure("staleConnection", "transport", "reload", 0)
	}
	write := method != http.MethodGet
	if readErr != nil || len(raw) > 4<<20 || !json.Valid(raw) {
		return MainProfileRecord{}, resourceResponseError(write)
	}
	var wire struct {
		MPAInstanceID       string          `json:"mpaInstanceId"`
		Revision            *int64          `json:"revision"`
		Digest              string          `json:"digest"`
		ETag                string          `json:"etag"`
		Profile             json.RawMessage `json:"profile"`
		SourceProfileID     *string         `json:"sourceProfileId"`
		SourceProfileDigest *string         `json:"sourceProfileDigest"`
		UpdatedBy           string          `json:"updatedBy"`
		CreatedAt           string          `json:"createdAt"`
	}
	if json.Unmarshal(raw, &wire) != nil || wire.MPAInstanceID != instanceID || wire.Revision == nil ||
		*wire.Revision < 1 || *wire.Revision > 9999999999 || wire.Digest == "" ||
		wire.ETag != `"`+strconv.FormatInt(*wire.Revision, 10)+`"` ||
		(response.Header.Get("ETag") != "" && response.Header.Get("ETag") != wire.ETag) {
		return MainProfileRecord{}, resourceResponseError(write)
	}
	var profileWire struct {
		AgentsMD *string            `json:"agentsMd"`
		Model    json.RawMessage    `json:"model"`
		Skills   []MainProfileSkill `json:"skills"`
	}
	// Refuse unknown profile fields rather than silently dropping settings on
	// a newer Runtime when this client later sends a replacement profile.
	if !mainProfileStrictDecode(wire.Profile, &profileWire) || profileWire.AgentsMD == nil || len(profileWire.Model) == 0 {
		return MainProfileRecord{}, resourceResponseError(write)
	}
	var model *MainProfileModel
	if !mainProfileStrictDecode(profileWire.Model, &model) {
		return MainProfileRecord{}, resourceResponseError(write)
	}
	profile := MainProfile{AgentsMD: *profileWire.AgentsMD, Model: model, Skills: profileWire.Skills}
	if !mainProfileValid(profile) || !mainProfileOptionalIdentifier(wire.SourceProfileID) ||
		!mainProfileOptionalIdentifier(wire.SourceProfileDigest) {
		return MainProfileRecord{}, resourceResponseError(write)
	}
	if err := s.resourceEpochError(epoch); err != nil {
		return MainProfileRecord{}, err
	}
	return MainProfileRecord{
		MPAInstanceID: wire.MPAInstanceID, Revision: *wire.Revision, Digest: wire.Digest, ETag: wire.ETag,
		Profile: profile, SourceProfileID: wire.SourceProfileID, SourceProfileDigest: wire.SourceProfileDigest,
		UpdatedBy: wire.UpdatedBy, CreatedAt: wire.CreatedAt,
	}, nil
}

func mainProfileStrictDecode(raw []byte, output any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(output) == nil && decoder.Decode(new(any)) == io.EOF
}

func mainProfileIdentifier(value string) bool {
	return utf8.ValidString(value) && value != "" && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) <= 512
}

func mainProfileOptionalIdentifier(value *string) bool {
	return value == nil || mainProfileIdentifier(*value)
}

func mainProfileValid(profile MainProfile) bool {
	if !resourceInstructionsValid(profile.AgentsMD) || profile.Skills == nil || len(profile.Skills) > 32 ||
		(profile.Model != nil && !mainProfileIdentifier(profile.Model.ID)) {
		return false
	}
	total := utf8.RuneCountInString(profile.AgentsMD)
	names := make(map[string]bool)
	for _, skill := range profile.Skills {
		name := strings.ToLower(skill.Name)
		if !mainProfileSkillName.MatchString(skill.Name) || names[name] || !mainProfileIdentifier(skill.Version) ||
			strings.EqualFold(skill.Version, "latest") || strings.ContainsAny(skill.Version, "*^~<>= ") ||
			skill.Content == "" || !resourceInstructionsValid(skill.Content) {
			return false
		}
		names[name] = true
		total += utf8.RuneCountInString(skill.Content)
	}
	return total <= resourceInstructionLimit
}

func mainProfileFailureStatus(err *dto.SafeError) MainProfileStatus {
	switch err.Code {
	case "main_profile_not_found", "main_profile_runtime_unconfigured":
		return MainProfileUnconfigured
	}
	if err.HTTPStatus == 405 {
		return MainProfileUnsupported
	}
	if err.HTTPStatus == 404 {
		// connection currently strips unknown provider codes, so the two 404
		// meanings cannot be distinguished without a transport contract change.
		return MainProfileUnsupportedOrUnconfigured
	}
	return MainProfileUnavailable
}

func (s *Service) mainProfileState(epoch uint64, scope string, record MainProfileRecord) MainProfileState {
	projection := cloneMainProfileRecord(record)
	return MainProfileState{
		Status: MainProfileConfigured, Record: &projection, ConnectionEpoch: epoch,
		snapshot: &mainProfileSnapshot{owner: s, epoch: epoch, scope: scope, record: record},
	}
}

func cloneMainProfileRecord(record MainProfileRecord) MainProfileRecord {
	if record.Profile.Model != nil {
		model := *record.Profile.Model
		record.Profile.Model = &model
	}
	record.Profile.Skills = append([]MainProfileSkill{}, record.Profile.Skills...)
	if record.SourceProfileID != nil {
		value := *record.SourceProfileID
		record.SourceProfileID = &value
	}
	if record.SourceProfileDigest != nil {
		value := *record.SourceProfileDigest
		record.SourceProfileDigest = &value
	}
	return record
}
