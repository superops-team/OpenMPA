package usage

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"strings"
	"testing"
)

const skillTestDocument = "---\nname: review-code\ndescription: |\n  Review code changes\n  and tests\n---\n# Code review\n"

func skillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	out := new(bytes.Buffer)
	writer := zip.NewWriter(out)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte(body))
	}
	if writer.Close() != nil {
		t.Fatal("zip close")
	}
	return out.Bytes()
}
func TestSkillArchiveAcceptsZipWrapperAndStandaloneMarkdown(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{{"review.zip", skillZip(t, map[string]string{"review/SKILL.md": skillTestDocument, "review/scripts/check.sh": "echo ok"})}, {"SKILL.md", []byte(skillTestDocument)}} {
		out, err := validateSkillArchive(tc.name, tc.data)
		if err != nil || out.Name != "review-code" || !strings.Contains(out.Description, "and tests") {
			t.Fatalf("valid archive rejected: %+v %v", out, err)
		}
	}
}
func TestSkillArchiveRejectsUnsafeAndInvalidPackages(t *testing.T) {
	for _, files := range []map[string]string{
		{"../SKILL.md": skillTestDocument}, {"/SKILL.md": skillTestDocument}, {"a/SKILL.md": skillTestDocument, "b/readme": "x"}, {"SKILL.md": skillTestDocument, "skill.md": skillTestDocument}, {"SKILL.md": "# No metadata"}, {"SKILL.md": "---\nname: bad_name\ndescription: example\n---"}, {"SKILL.md": "---\nname: okay\ndescription: <script>bad</script>\n---"},
	} {
		if _, err := validateSkillArchive("test.zip", skillZip(t, files)); err == nil {
			t.Fatalf("unsafe archive accepted: %+v", files)
		}
	}
	if _, err := validateSkillArchive("private.txt", []byte(skillTestDocument)); err == nil {
		t.Fatal("unsupported file accepted")
	}
}
func TestSkillUploadPublishesOnlyOnceToBoundSpace(t *testing.T) {
	var s *Service
	created, published, stored := 0, 0, 0
	createdID := ""
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Query().Get("Action") {
		case "GetSkillSpace":
			return `{"Result":{"Id":"mounted","Name":"Worker skills","ProjectName":"default"}}`
		case "ListSkillsBySkillSpace":
			return `{"Result":{"Items":[],"TotalCount":0}}`
		case "ListSkillsBySpaceId":
			return `{"Result":{"Items":[]}}`
		case "CreateSkill":
			created++
			if body["TosUrl"] != "https://storage.invalid/package.zip" || body["Name"] != "review-code" || body["SkillSpaces"].([]any)[0] != "mounted" {
				t.Errorf("incorrect CreateSkill payload")
			}
			createdID = "skill-new"
			return `{"Result":{"Id":"skill-new"}}`
		case "ListSkillVersions":
			if body["Id"] != createdID {
				t.Error("unrelated skill queried")
			}
			return `{"Result":{"Items":[{"Version":"v1","Status":"Running"}]}}`
		case "PublishSkillToSkillSpace":
			published++
			if body["SkillSpaces"].([]any)[0] != "mounted" || body["Skills"].([]any)[0].(map[string]any)["Version"] != "v1" {
				t.Error("wrong publication")
			}
			return `{"Result":{}}`
		}
		t.Errorf("unexpected action %s", r.URL.Query().Get("Action"))
		return `{}`
	})
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	s.putSkillArchive = func(ctx context.Context, region, project string, data []byte) (cloud.SkillArchiveObject, *dto.SafeError) {
		stored++
		if region != "cn-beijing" || project != "default" {
			t.Error("wrong storage scope")
		}
		return cloud.SkillArchiveObject{URL: "https://storage.invalid/package.zip", Bucket: "private"}, nil
	}
	req := SkillUploadRequest{ConnectionEpoch: epoch, OperationID: "upload-1", SpaceID: "mounted", Filename: "SKILL.md", ArchiveBase64: base64.StdEncoding.EncodeToString([]byte(skillTestDocument))}
	out, err := s.UploadSkill(context.Background(), req)
	if err != nil || out.Status != "processing" {
		t.Fatalf("upload: %+v %v", out, err)
	}
	out, err = s.ReadSkillUpload(context.Background(), SkillUploadReadRequest{epoch, "upload-1"})
	if err != nil || out.Status != "published" || out.Version != "v1" {
		t.Fatalf("publish: %+v %v", out, err)
	}
	s.ReadSkillUpload(context.Background(), SkillUploadReadRequest{epoch, "upload-1"})
	s.UploadSkill(context.Background(), req)
	if created != 1 || published != 1 || stored != 1 {
		t.Fatalf("duplicate writes: %d %d %d", created, published, stored)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "storage.invalid") || strings.Contains(string(raw), "private") {
		t.Fatal("storage internals exposed to renderer")
	}
}
func TestSkillUploadRejectsUnboundSpaceBeforeStorage(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	s.cloud = resourceTestCloud(t, func(r *http.Request) string { return `{"Result":{"Id":"mounted","Name":"Worker skills"}}` })
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	s.putSkillArchive = func(context.Context, string, string, []byte) (cloud.SkillArchiveObject, *dto.SafeError) {
		t.Error("unbound package reached storage")
		return cloud.SkillArchiveObject{}, nil
	}
	_, err := s.UploadSkill(context.Background(), SkillUploadRequest{ConnectionEpoch: epoch, OperationID: "op", SpaceID: "other", Filename: "SKILL.md", ArchiveBase64: base64.StdEncoding.EncodeToString([]byte(skillTestDocument))})
	if err == nil || err.Code != "resourceNotVisible" {
		t.Fatalf("unbound upload: %v", err)
	}
}

func TestSkillUploadLostPublishReceiptReadsRelationWithoutRepublishing(t *testing.T) {
	s, epoch := resourceTestService(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, resourceTestCard) })
	published := 0
	s.cloud = resourceTestCloud(t, func(r *http.Request) string {
		switch r.URL.Query().Get("Action") {
		case "GetSkillSpace":
			return `{"Result":{"Id":"mounted","Name":"Worker skills"}}`
		case "ListSkillVersions":
			return `{"Result":{"Items":[{"Version":"v1","Status":"Running"}]}}`
		case "PublishSkillToSkillSpace":
			published++
			return `{`
		case "ListSkillsBySkillSpace":
			return `{"Result":{"Items":[{"SkillId":"skill-new","SkillName":"review-code","Version":"v1"}],"TotalCount":1}}`
		}
		t.Errorf("unexpected action %s", r.URL.Query().Get("Action"))
		return `{}`
	})
	s.target = func() deployment.Target {
		return deployment.Target{RuntimeID: "r-runtime", Region: "cn-beijing", ConnectionEpoch: epoch, PlatformEpoch: s.cloud.Epoch()}
	}
	s.skillUploads.operations["lost-receipt"] = &skillUploadOperation{
		View:  SkillUploadView{OperationID: "lost-receipt", SpaceID: "mounted", Name: "review-code", SkillID: "skill-new", Status: "processing"},
		Epoch: epoch, PlatformEpoch: s.cloud.Epoch(), Scope: s.conn.State().Scope, created: true,
	}
	request := SkillUploadReadRequest{epoch, "lost-receipt"}
	out, err := s.ReadSkillUpload(context.Background(), request)
	if err == nil || out.Status != "unknown" {
		t.Fatalf("lost receipt: %+v %v", out, err)
	}
	out, err = s.ReadSkillUpload(context.Background(), request)
	if err != nil || out.Status != "published" {
		t.Fatalf("relation recovery: %+v %v", out, err)
	}
	s.ReadSkillUpload(context.Background(), request)
	if published != 1 {
		t.Fatalf("repeated publication: %d", published)
	}
}
