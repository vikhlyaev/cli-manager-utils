// Package fsutil содержит безопасную запись файлов.
package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFileAtomic записывает data в path так, что файл никогда не остаётся
// наполовину записанным: данные пишутся во временный файл в том же каталоге,
// сбрасываются на диск и только потом заменяют файл переименованием.
//
//   - Если path — симлинк, заменяется файл, на который он указывает, а сама
//     ссылка остаётся (конфиг может жить в репозитории dotfiles).
//   - Права существующего файла сохраняются; perm применяется только к новому.
//   - check (может быть nil) вызывается с путём временного файла перед заменой.
//     Если он вернул ошибку, временный файл удаляется, а прежний не меняется.
func WriteFileAtomic(path string, data []byte, perm os.FileMode, check func(tmpPath string) error) (err error) {
	target, err := resolveTarget(path)
	if err != nil {
		return err
	}
	if fi, statErr := os.Stat(target); statErr == nil {
		perm = fi.Mode().Perm()
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	if check != nil {
		if err = check(tmpPath); err != nil {
			return err
		}
	}
	if err = os.Rename(tmpPath, target); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// resolveTarget возвращает путь к реальному файлу: для симлинка — к цели
// ссылки, для отсутствующего файла — сам path.
func resolveTarget(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	// Висячая ссылка: пишем туда, куда она указывает.
	if link, lerr := os.Readlink(path); lerr == nil {
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		return link, nil
	}
	return path, nil
}

// syncDir сбрасывает на диск запись каталога, чтобы переименование пережило
// сбой питания. Ошибки не критичны: данные в любом случае целы.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
