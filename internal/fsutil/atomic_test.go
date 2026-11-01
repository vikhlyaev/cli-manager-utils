package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "f.txt") // каталога ещё нет
	if err := WriteFileAtomic(path, []byte("one"), 0o644, nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("новый файл: ожидались права 0644, получено %v", fi.Mode().Perm())
	}
	if err := WriteFileAtomic(path, []byte("two"), 0o600, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "two" {
		t.Fatalf("ожидалось 'two', получено %q", got)
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

func TestWriteFileAtomicCheckFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	var seen string
	errBad := errors.New("плохой файл")
	err := WriteFileAtomic(path, []byte("new"), 0o644, func(tmp string) error {
		b, _ := os.ReadFile(tmp)
		seen = string(b)
		return errBad
	})
	if !errors.Is(err, errBad) {
		t.Fatalf("ожидалась ошибка проверки, получено %v", err)
	}
	if seen != "new" {
		t.Errorf("проверка должна видеть новые данные, получено %q", seen)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Fatalf("при провале проверки файл не должен меняться, получено %q", got)
	}
	assertNoTempFiles(t, dir)
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("остался временный файл %s", e.Name())
		}
	}
}

// TestWriteFileAtomicFollowsSymlink — запись через симлинк меняет целевой файл,
// а сама ссылка остаётся ссылкой.
func TestWriteFileAtomicFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(link, []byte("new"), 0o644, nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("симлинк заменён обычным файлом")
	}
	if got, _ := os.ReadFile(real); string(got) != "new" {
		t.Fatalf("целевой файл должен обновиться, получено %q", got)
	}
	assertNoTempFiles(t, dir)
	assertNoTempFiles(t, filepath.Dir(real))
}

// TestWriteFileAtomicKeepsExistingPerm — права существующего файла не
// сбрасываются (конфиг с секретами может быть 0600).
func TestWriteFileAtomicKeepsExistingPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o644, nil); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("права должны остаться 0600, получено %v", fi.Mode().Perm())
	}
}
