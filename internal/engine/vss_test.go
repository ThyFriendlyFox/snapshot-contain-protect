package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vssRunner fakes the Windows commands so the backend's command construction
// is tested on any host. Only the live gate needs Windows.
type vssRunner struct {
	calls   []string
	shadows int
	failOn  string
}

func (r *vssRunner) run(_ context.Context, name string, args ...string) ([]byte, error) {
	line := name + " " + strings.Join(args, " ")
	r.calls = append(r.calls, line)
	if r.failOn != "" && strings.Contains(line, r.failOn) {
		return nil, errors.New("refused")
	}
	switch {
	case strings.Contains(line, "IsInRole"):
		return []byte("True\r\n"), nil
	case strings.Contains(line, "Win32_ShadowCopy').Create"):
		r.shadows++
		// A real provider prints its banner before the answer.
		return []byte("\r\nWindows PowerShell\r\n{SHADOW-00" +
			string(rune('0'+r.shadows)) + "}|\\\\?\\GLOBALROOT\\Device\\HarddiskVolumeShadowCopy" +
			string(rune('0'+r.shadows)) + "\r\n"), nil
	case strings.HasPrefix(line, "cmd /c mklink"):
		// mklink makes the directory. Fake that much so the manifest and the
		// differ have something to read.
		_ = os.MkdirAll(args[len(args)-2], 0o755)
		return nil, nil
	case strings.HasPrefix(line, "cmd /c rmdir"):
		_ = os.Remove(args[len(args)-1])
		return nil, nil
	}
	return nil, nil
}

func (r *vssRunner) find(substr string) string {
	for _, c := range r.calls {
		if strings.Contains(c, substr) {
			return c
		}
	}
	return ""
}

func newFakeVSS(t *testing.T) (*VSS, *vssRunner, string) {
	t.Helper()
	base := t.TempDir()
	work := filepath.Join(base, "work")
	writeFile(t, filepath.Join(work, "a.txt"), "a")

	r := &vssRunner{}
	v := NewVSS(filepath.Join(base, "snapshots"))
	v.run = r.run
	if err := os.MkdirAll(v.root, 0o755); err != nil {
		t.Fatal(err)
	}
	return v, r, work
}

func TestVSSCreateMakesAShadowCopyAndMountsIt(t *testing.T) {
	v, r, work := newFakeVSS(t)
	handle, err := v.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}

	volume, err := VolumeOf(work)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.find("Win32_ShadowCopy').Create"); !strings.Contains(got, "Create('"+volume+"'") {
		t.Fatalf("create call = %q, want the volume %q", got, volume)
	}
	// The mount needs the trailing separator; mklink refuses the device
	// path without it.
	mount := r.find("mklink")
	if !strings.HasSuffix(mount, `HarddiskVolumeShadowCopy1\`) {
		t.Fatalf("mklink call = %q, want a trailing separator", mount)
	}
	if !strings.Contains(mount, filepath.Join(handle, "vol0")) {
		t.Fatalf("mklink call = %q, want the mount inside the handle", mount)
	}
}

func TestVSSManifestPointsEachSourceIntoItsMount(t *testing.T) {
	v, _, work := newFakeVSS(t)
	handle, err := v.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(handle)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Shadows) != 1 || m.Shadows[0].Dir != "vol0" {
		t.Fatalf("shadows = %+v, want 1 mounted at vol0", m.Shadows)
	}
	if m.Shadows[0].ID == "" {
		t.Fatal("the manifest holds no shadow copy identifier, so nothing can delete it")
	}
	if len(m.Sources) != 1 {
		t.Fatalf("sources = %+v, want 1", m.Sources)
	}
	// The source must resolve to a subpath of the mount, which is what lets
	// the shared differ walk a volume-wide snapshot as if it were a subtree.
	if !strings.HasPrefix(m.Sources[0].Dir, "vol0") {
		t.Fatalf("source dir = %q, want it inside vol0", m.Sources[0].Dir)
	}
}

func TestVSSDeleteRemovesTheMountThenTheShadowCopy(t *testing.T) {
	v, r, work := newFakeVSS(t)
	handle, err := v.Create(context.Background(), "01AAA", []string{work})
	if err != nil {
		t.Fatal(err)
	}
	r.calls = nil
	if err := v.Delete(context.Background(), handle); err != nil {
		t.Fatal(err)
	}

	var rmdirAt, deleteAt = -1, -1
	for i, c := range r.calls {
		if strings.HasPrefix(c, "cmd /c rmdir") {
			rmdirAt = i
		}
		if strings.Contains(c, "Remove-CimInstance") {
			deleteAt = i
		}
	}
	if rmdirAt < 0 {
		t.Fatal("the mount was never removed")
	}
	if deleteAt < 0 {
		t.Fatal("the shadow copy was never deleted")
	}
	// rmdir removes the link. RemoveAll would walk into the shadow copy, and
	// deleting the copy first would leave a link pointing at nothing.
	if rmdirAt > deleteAt {
		t.Fatalf("the shadow copy was deleted before its mount: %v", r.calls)
	}
	if _, err := os.Stat(handle); !os.IsNotExist(err) {
		t.Fatal("the handle survived delete")
	}
}

func TestVSSCleansUpAfterAFailedMount(t *testing.T) {
	v, r, work := newFakeVSS(t)
	r.failOn = "mklink"

	if _, err := v.Create(context.Background(), "01AAA", []string{work}); err == nil {
		t.Fatal("a failed mount reported success")
	}
	// The shadow copy exists even though the mount failed. Leaving it would
	// consume the volume's shadow storage with nothing able to find it.
	if r.find("Remove-CimInstance") == "" {
		t.Fatalf("the orphaned shadow copy was never deleted: %v", r.calls)
	}
	if _, err := os.Stat(filepath.Join(v.root, "01AAA")); !os.IsNotExist(err) {
		t.Fatal("the failed handle was left behind")
	}
}

func TestVSSRefusesWithoutElevation(t *testing.T) {
	base := t.TempDir()
	v := NewVSS(base)
	v.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("False"), nil
	}
	// Available also checks the platform, so off Windows this asserts the
	// elevation check in isolation.
	elevated, err := v.elevated(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if elevated {
		t.Fatal("a non-administrator was reported as elevated")
	}
}

func TestLastLineIgnoresABanner(t *testing.T) {
	got := lastLine("\r\nWindows PowerShell\r\n{ID}|\\\\?\\GLOBALROOT\\Device\\X\r\n\r\n")
	if got != `{ID}|\\?\GLOBALROOT\Device\X` {
		t.Fatalf("lastLine = %q", got)
	}
}
