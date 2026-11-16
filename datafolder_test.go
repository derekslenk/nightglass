package main

import (
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFrom(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "APOD")
	d := &dataFolder{dir: filepath.Join(root, "Nightglass")}

	// Nothing to migrate: no error, nothing created.
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.dir); err == nil {
		t.Fatal("created new dir with nothing to migrate")
	}

	// Old folder moves, settings survive, log is renamed.
	os.Mkdir(oldDir, 0o755)
	if err := (&dataFolder{dir: oldDir}).saveSettings(settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(oldDir, "apod.log"), []byte("old"), 0o644)
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if got := d.loadSettings(); got.BarStyle != BarsCrisp {
		t.Fatalf("migrated style = %q, want crisp", got.BarStyle)
	}
	if _, err := os.Stat(d.logPath()); err != nil {
		t.Fatalf("log not renamed: %v", err)
	}
	if _, err := os.Stat(oldDir); err == nil {
		t.Fatal("old dir still exists")
	}

	// New folder already exists: an old one reappearing is left alone.
	os.Mkdir(oldDir, 0o755)
	if err := d.migrateFrom(oldDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); err != nil {
		t.Fatal("touched old dir although new dir existed")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	d := &dataFolder{dir: t.TempDir()}
	if got := d.loadSettings(); got.BarStyle != BarsSoft {
		t.Fatalf("default style = %q, want soft", got.BarStyle)
	}
	if err := d.saveSettings(settings{BarStyle: BarsCrisp}); err != nil {
		t.Fatal(err)
	}
	if got := d.loadSettings(); got.BarStyle != BarsCrisp {
		t.Fatalf("reloaded style = %q, want crisp", got.BarStyle)
	}
}

func TestRendersKeepsOldFilesWhenOneFails(t *testing.T) {
	d := &dataFolder{dir: t.TempDir()}
	rdir := filepath.Join(d.dir, "rendered")
	os.MkdirAll(rdir, 0o755)
	old := filepath.Join(rdir, "2026-11-07_32x32_v5_soft.png") // what the monitor shows now
	os.WriteFile(old, []byte("x"), 0o644)
	bad := filepath.Join(d.dir, "2026-11-08.png")
	os.WriteFile(bad, []byte("junk"), 0o644)
	out, err := d.renders(map[string]string{"side": bad}, []Monitor{{ID: "side", Width: 32, Height: 32}}, BarsSoft)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := out["side"]; ok {
		t.Errorf("failed monitor still in result: %s", p)
	}
	if _, err := os.Stat(old); err != nil {
		t.Error("old render of the skipped monitor was deleted")
	}
}

func TestRendersWriteFailureIsAnError(t *testing.T) {
	d := &dataFolder{dir: t.TempDir()}
	good := filepath.Join(d.dir, "2026-11-08.png")
	f, _ := os.Create(good)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 8)))
	f.Close()
	// A directory in the way of the temp file makes the render write fail.
	os.MkdirAll(filepath.Join(d.dir, "rendered", "2026-11-08_32x32_v5_soft.png.part"), 0o755)
	_, err := d.renders(map[string]string{"m": good}, []Monitor{{ID: "m", Width: 32, Height: 32}}, BarsSoft)
	if err == nil {
		t.Fatal("a failed render write was reported as success")
	}
	if _, err := os.Stat(good); err != nil {
		t.Error("good picture was deleted after a write failure")
	}
}

func TestWriteAtomicFailureLeavesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.bin")
	boom := errors.New("boom")
	err := writeAtomic(p, func(w io.Writer) error {
		w.Write([]byte("partial"))
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	for _, f := range []string{p, p + ".part"} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s left behind after a failed write", filepath.Base(f))
		}
	}
}

func TestWriteAtomicRenameFailureLeavesNoPart(t *testing.T) {
	p := t.TempDir() // a directory in the way makes the final rename fail
	err := writeAtomic(p, func(w io.Writer) error { _, err := w.Write([]byte("x")); return err })
	if err == nil {
		t.Fatal("expected rename onto a directory to fail")
	}
	if _, err := os.Stat(p + ".part"); err == nil {
		t.Error(".part left behind after a failed rename")
	}
}
