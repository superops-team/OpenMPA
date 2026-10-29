package mpalifecycle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

func archiveBytes(t *testing.T, extra *tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	writer := tar.NewWriter(gz)
	for _, name := range []string{"engine.py", "manifest.json", "python/bin/python3.12"} {
		if err := writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0700, Size: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if extra != nil {
		if err := writer.WriteHeader(extra); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestArchiveRejectsTraversalLinksAndDuplicates(t *testing.T) {
	for _, h := range []tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0600},
		{Name: "/absolute", Typeflag: tar.TypeReg, Mode: 0600},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/tmp"},
		{Name: "link", Typeflag: tar.TypeLink, Linkname: "engine.py"},
		{Name: "engine.py", Typeflag: tar.TypeReg, Mode: 0600},
	} {
		t.Run(h.Name+string(h.Typeflag), func(t *testing.T) {
			gz, err := gzip.NewReader(bytes.NewReader(archiveBytes(t, &h)))
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			if extractEngine(tar.NewReader(gz), t.TempDir()) == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}
func TestArchivePreparationIsPrivateAndReusable(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "engine.tar.gz")
	if err := os.WriteFile(archive, archiveBytes(t, nil), 0600); err != nil {
		t.Fatal(err)
	}
	python, script, err := prepareArchive(archive, filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(python)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("bad executable permissions %v %v", info, err)
	}
	second, _, err := prepareArchive(archive, filepath.Join(root, "cache"))
	if err != nil || second != python {
		t.Fatal("cache not reused", err)
	}
	if !strings.HasSuffix(script, "engine.py") {
		t.Fatal(script)
	}
}
func helper(t *testing.T) (string, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python test interpreter unavailable")
	}
	script := filepath.Join(t.TempDir(), "fake.py")
	code := `import sys,json,time,os
initial=json.loads(sys.stdin.readline())
assert initial['credentials'][1]=='SECRET'
print('ignored module import noise')
print(json.dumps({'id':'init','ok':True,'value':{'ready':True}}),flush=True)
for line in sys.stdin:
 request=json.loads(line)
 op=request['operation']
 if op=='shutdown':break
 if op=='wait':time.sleep(.2)
 if op=='failure':
  print('SECRET SHOULD NEVER BE VISIBLE',file=sys.stderr,flush=True)
  value={'id':request['id'],'ok':False,'error':{'code':'denied','httpStatus':403}}
 elif op=='readDeletion':value={'id':request['id'],'ok':True,'value':{'operationId':'op','runtimeId':'r-one','region':'cn-beijing','state':'succeeded','stage':'complete'}}
 else:value={'id':request['id'],'ok':True,'value':{'configured':True,'region':'cn-beijing','inherited':os.environ.get('OPENMPA_TEST_SECRET','')}}
 print(json.dumps(value),flush=True)
`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	return python, script
}
func TestPrivateProtocolCancellationAndRedaction(t *testing.T) {
	t.Setenv("OPENMPA_TEST_SECRET", "DO_NOT_INHERIT")
	python, script := helper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, failure := launchEngine(ctx, python, script, map[string]any{"credentials": []string{"AK", "SECRET", ""}})
	if failure != nil {
		t.Fatal(failure)
	}
	defer p.close()
	var output map[string]any
	if err := p.call(ctx, "config", map[string]any{}, &output); err != nil {
		t.Fatal(err)
	}
	if output["configured"] != true || output["inherited"] != "" {
		t.Fatal(output)
	}
	if err := p.call(ctx, "failure", map[string]any{}, &output); err == nil || err.Code != "denied" || strings.Contains(err.Message, "SECRET") {
		t.Fatalf("unsafe failure %+v", err)
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := p.call(short, "wait", map[string]any{}, &output); err == nil || err.Code != "outcomeUnknown" {
		t.Fatalf("lost request not uncertain: %+v", err)
	}
	// A late receipt must not corrupt the next request.
	if err := p.call(ctx, "config", map[string]any{}, &output); err != nil {
		t.Fatal(err)
	}
}

type identityTransport struct{}

func (identityTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"Result":{"AccountId":"one","IdentityType":"User","UserId":"user"}}`))}, nil
}
func TestAccountEpochAndAuthenticationAreCheckedBeforeEngineStartup(t *testing.T) {
	platform := cloud.NewWithTransport(identityTransport{})
	started := 0
	s := New(platform, func() (string, error) { started++; return t.TempDir(), nil }, func() (string, error) { return "absent", nil }, nil)
	defer Shutdown(s)
	if value := s.Config(context.Background(), Scope{Region: "cn-beijing"}); value.OK || value.Error.Code != "authRequired" {
		t.Fatal(value)
	}
	state, err := platform.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK", SecretKey: "SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	if value := s.Config(context.Background(), Scope{PlatformEpoch: state.Epoch + 1, Region: "cn-beijing"}); value.OK || value.Error.Code != "staleConnection" {
		t.Fatal(value)
	}
	if value := s.Delete(context.Background(), DeleteRequest{PlatformEpoch: state.Epoch, Region: "cn-beijing", RuntimeID: "r-one", Fingerprint: strings.Repeat("a", 64)}); value.OK || value.Error.Code != "invalidInput" {
		t.Fatal(value)
	}
	if started != 0 {
		t.Fatal("untrusted call started the engine")
	}
}
func TestBundledEngineStartsFromExtractedResources(t *testing.T) {
	archive := os.Getenv("OPENMPA_ENGINE_ARCHIVE")
	if archive == "" {
		t.Skip("set OPENMPA_ENGINE_ARCHIVE for the packaged engine smoke test")
	}
	platform := cloud.NewWithTransport(identityTransport{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	state, failure := platform.SignIn(ctx, dto.SignInRequest{Region: "cn-beijing", AccessKey: "TEST_AK", SecretKey: "TEST_SK"})
	if failure != nil {
		t.Fatal(failure)
	}
	root := t.TempDir()
	s := New(platform, func() (string, error) { return root, nil }, func() (string, error) { return archive, nil }, nil)
	defer Shutdown(s)
	config := s.Config(ctx, Scope{PlatformEpoch: state.Epoch, Region: "cn-beijing"})
	if !config.OK || !config.Value.Configured || config.Value.PostgresMode != "auto" {
		b, _ := json.Marshal(config)
		t.Fatalf("bad bundled configuration %s", b)
	}
	tasks := s.ListCreations(ctx, Scope{PlatformEpoch: state.Epoch, Region: "cn-beijing"})
	if !tasks.OK || len(tasks.Value.Tasks) != 0 {
		t.Fatalf("journal bootstrap failed %+v", tasks)
	}
	s.mu.Lock()
	python := s.process.command.Path
	s.mu.Unlock()
	imports := exec.CommandContext(ctx, python, "-I", "-c", `import volcenginesdkaidap,volcenginesdkecs,volcenginesdkvpc
from volcenginesdkcore.universal import UniversalApi,UniversalInfo
assert '.zip/' in volcenginesdkaidap.__file__
assert '.zip/' in volcenginesdkecs.__file__
assert '.zip/' in volcenginesdkvpc.__file__
print('packaged PG, network and universal SDK imports verified')`)
	if output, err := imports.CombinedOutput(); err != nil {
		t.Fatalf("packaged SDK imports failed: %v %s", err, output)
	}
}

func TestLateEngineReceiptCannotCrossAccountEpoch(t *testing.T) {
	platform := cloud.NewWithTransport(identityTransport{})
	state, failure := platform.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK", SecretKey: "SECRET"})
	if failure != nil {
		t.Fatal(failure)
	}
	python, script := helper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, failure := launchEngine(ctx, python, script, map[string]any{"credentials": []string{"AK", "SECRET", ""}})
	if failure != nil {
		t.Fatal(failure)
	}
	s := New(platform, nil, nil, nil)
	s.mu.Lock()
	s.process = p
	s.epoch = state.Epoch
	s.mu.Unlock()
	defer Shutdown(s)
	result := make(chan *dto.SafeError, 1)
	go func() {
		var output Config
		result <- s.call(ctx, state.Epoch, "cn-beijing", "wait", map[string]any{}, &output)
	}()
	deadline := time.Now().Add(time.Second)
	for p.next.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if p.next.Load() == 0 {
		t.Fatal("request never started")
	}
	platform.SignOut()
	if err := <-result; err == nil || err.Code != "staleConnection" {
		t.Fatalf("old receipt accepted after signout: %+v", err)
	}
}

func TestAcceptedDeletionIsMonitoredWithoutAWebView(t *testing.T) {
	platform := cloud.NewWithTransport(identityTransport{})
	state, failure := platform.SignIn(context.Background(), dto.SignInRequest{Region: "cn-beijing", AccessKey: "AK", SecretKey: "SECRET"})
	if failure != nil {
		t.Fatal(failure)
	}
	python, script := helper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	p, failure := launchEngine(ctx, python, script, map[string]any{"credentials": []string{"AK", "SECRET", ""}})
	if failure != nil {
		t.Fatal(failure)
	}
	notified := make(chan string, 2)
	s := New(platform, nil, nil, func(epoch uint64, region, id string) {
		if epoch != state.Epoch || region != "cn-beijing" {
			t.Error("wrong completion scope")
		}
		notified <- id
	})
	s.mu.Lock()
	s.process = p
	s.epoch = state.Epoch
	s.mu.Unlock()
	defer Shutdown(s)
	s.monitorDeletion(state.Epoch, "cn-beijing", "op")
	s.monitorDeletion(state.Epoch, "cn-beijing", "op")
	select {
	case id := <-notified:
		if id != "r-one" {
			t.Fatal(id)
		}
	case <-ctx.Done():
		t.Fatal("background deletion was not observed")
	}
	if p.next.Load() != 1 {
		t.Fatal("duplicate monitors issued additional requests")
	}
}
