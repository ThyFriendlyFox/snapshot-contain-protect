package engine

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// VSS is the Windows backend. It makes a Volume Shadow Copy of every volume
// the workset touches. It never calls System Restore, which takes minutes and
// records registry state a rollback does not need.
//
// A shadow copy is not a path. It answers as a device such as
// `\\?\GLOBALROOT\Device\HarddiskVolumeShadowCopy1`, and files cannot be read
// through that device directly. A directory symlink to it can be read, and
// the trailing separator is required. So a handle here is a mount: Create
// makes the shadow copy and then the link, and Delete removes both.
//
// Every command goes through a Runner, so the command construction is tested
// on any host. Only the live gate needs Windows.
type VSS struct {
	root string
	run  Runner
}

// NewVSS returns a VSS backend that keeps its mounts under root.
func NewVSS(root string) *VSS { return &VSS{root: root, run: execRunner} }

func (v *VSS) Name() string { return "vss" }

func (v *VSS) Available() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("%w: the vss backend needs windows", ErrUnavailable)
	}
	if _, err := exec.LookPath("powershell"); err != nil {
		return fmt.Errorf("%w: powershell not found", ErrUnavailable)
	}
	elevated, err := v.elevated(context.Background())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !elevated {
		// State the requirement, not a stack trace. A shadow copy is an
		// administrator operation and no flag changes that.
		return fmt.Errorf("%w: the vss backend needs Administrator; run the daemon elevated", ErrUnavailable)
	}
	return nil
}

// elevated reports whether this process can make a shadow copy at all.
func (v *VSS) elevated(ctx context.Context) (bool, error) {
	out, err := v.run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)`)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(string(out)), "true"), nil
}

func (v *VSS) Create(ctx context.Context, id string, sources []string) (string, error) {
	if err := checkSources(v.root, sources); err != nil {
		return "", err
	}
	volumes, err := VolumesOf(sources)
	if err != nil {
		return "", err
	}

	handle := filepath.Join(v.root, id)
	if err := mkdirAll(handle); err != nil {
		return "", err
	}

	m := manifest{}
	for i, volume := range volumes {
		shadowID, device, err := v.createShadow(ctx, volume)
		if err != nil {
			v.cleanup(ctx, handle, m)
			return "", err
		}
		mount := filepath.Join(handle, volumeDir(i))
		if err := v.mount(ctx, device, mount); err != nil {
			// The shadow copy exists but has no mount. Record it so cleanup
			// can still find and delete it.
			m.Shadows = append(m.Shadows, shadow{ID: shadowID, Volume: volume, Dir: volumeDir(i)})
			v.cleanup(ctx, handle, m)
			return "", err
		}
		m.Shadows = append(m.Shadows, shadow{ID: shadowID, Volume: volume, Dir: volumeDir(i)})
	}

	// Each source becomes a subpath inside its volume's mount, so the shared
	// differ and the byte-copy restore walk it the way they walk any handle.
	for _, src := range sources {
		dir, err := mountedSubpath(m, src)
		if err != nil {
			v.cleanup(ctx, handle, m)
			return "", err
		}
		m.Sources = append(m.Sources, source{Path: src, Dir: dir})
	}

	if err := writeManifest(handle, m); err != nil {
		v.cleanup(ctx, handle, m)
		return "", err
	}
	return handle, nil
}

// createShadow makes 1 shadow copy and returns its identifier and device.
func (v *VSS) createShadow(ctx context.Context, volume string) (id, device string, err error) {
	script := fmt.Sprintf(
		`$r = ([WMICLASS]'root\cimv2:Win32_ShadowCopy').Create('%s','ClientAccessible'); `+
			`if ($r.ReturnValue -ne 0) { Write-Error ('Win32_ShadowCopy.Create returned ' + $r.ReturnValue); exit 1 }; `+
			`$s = Get-CimInstance Win32_ShadowCopy | Where-Object { $_.ID -eq $r.ShadowID }; `+
			`Write-Output ($s.ID + '|' + $s.DeviceObject)`,
		psQuote(volume))
	out, err := v.run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return "", "", fmt.Errorf("shadow copy of %s: %w", volume, err)
	}
	id, device, ok := strings.Cut(lastLine(string(out)), "|")
	if !ok || id == "" || device == "" {
		return "", "", fmt.Errorf("shadow copy of %s: cannot read the identifier and device from %q", volume, out)
	}
	return id, device, nil
}

// mount links the shadow copy device into the handle. The trailing separator
// is required: mklink refuses the device path without it.
func (v *VSS) mount(ctx context.Context, device, mount string) error {
	_, err := v.run(ctx, "cmd", "/c", "mklink", "/d", mount, device+`\`)
	if err != nil {
		return fmt.Errorf("mount %s at %s: %w", device, mount, err)
	}
	return nil
}

func (v *VSS) Delete(ctx context.Context, handle string) error {
	m, err := readManifest(handle)
	if err != nil {
		// A handle with no manifest names no shadow copy. Remove what is
		// there and report nothing, so a prune can finish.
		return removeTree(handle)
	}
	v.cleanup(ctx, handle, m)
	return nil
}

// cleanup removes every mount and then every shadow copy the manifest names,
// then the handle itself. It reports nothing: each caller already has the
// error that brought it here.
func (v *VSS) cleanup(ctx context.Context, handle string, m manifest) {
	for _, s := range m.Shadows {
		mount := filepath.Join(handle, s.Dir)
		// rmdir removes the link, never what it points at. RemoveAll on a
		// directory symlink would walk into the shadow copy.
		_, _ = v.run(ctx, "cmd", "/c", "rmdir", mount)
	}
	for _, s := range m.Shadows {
		script := fmt.Sprintf(
			`Get-CimInstance Win32_ShadowCopy | Where-Object { $_.ID -eq '%s' } | Remove-CimInstance`,
			psQuote(s.ID))
		_, _ = v.run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	}
	_ = removeTree(handle)
}

func (v *VSS) Diff(ctx context.Context, from, to string) (Change, error) {
	// The mounts make a shadow copy look like any other tree.
	return diffHandles(ctx, from, to)
}

func (v *VSS) Restore(context.Context, string) error {
	// ROADMAP item 5. A restore here copies out of the mount into the live
	// paths; there is no volume-level revert, which would take the whole
	// disk back.
	return fmt.Errorf("%w: vss restore is not implemented", ErrUnavailable)
}

// mountedSubpath maps a source path to its place inside the handle.
func mountedSubpath(m manifest, src string) (string, error) {
	for _, s := range m.Shadows {
		rel, err := filepath.Rel(s.Volume, src)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if rel == "." {
			return s.Dir, nil
		}
		return filepath.Join(s.Dir, rel), nil
	}
	return "", fmt.Errorf("%w: no shadow copy covers %q", ErrBadSource, src)
}

func volumeDir(i int) string { return fmt.Sprintf("vol%d", i) }

// psQuote makes a value safe inside a single-quoted PowerShell string.
func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// lastLine returns the final non-empty line, so a banner before the answer
// does not become the answer.
func lastLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
