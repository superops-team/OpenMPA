package mpalifecycle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func prepareArchive(archive, root string) (string, string, error) {
	file, err := os.Open(archive)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", "", err
	}
	destination := filepath.Join(root, "engine-"+hex.EncodeToString(hash.Sum(nil))[:20])
	python := filepath.Join(destination, "python", "bin", "python3.12")
	script := filepath.Join(destination, "engine.py")
	if _, err = os.Stat(filepath.Join(destination, ".ready")); err == nil {
		return python, script, nil
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", "", err
	}
	temporary, err := os.MkdirTemp(root, ".engine-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(temporary)
	if _, err = file.Seek(0, 0); err != nil {
		return "", "", err
	}
	reader, err := gzip.NewReader(file)
	if err != nil {
		return "", "", err
	}
	defer reader.Close()
	if err = extractEngine(tar.NewReader(reader), temporary); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(filepath.Join(temporary, ".ready"), []byte("1"), 0600); err != nil {
		return "", "", err
	}
	if err = os.Rename(temporary, destination); err != nil {
		if _, existing := os.Stat(filepath.Join(destination, ".ready")); existing != nil {
			return "", "", err
		}
	}
	return python, script, nil
}

func extractEngine(reader *tar.Reader, root string) error {
	var total int64
	directories := map[string]bool{filepath.Clean(root): true}
	ensureDirectory := func(path string) error {
		if directories[path] {
			return nil
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		directories[path] = true
		return nil
	}
	for count := 0; ; count++ {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if count > 150000 || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) || header.Size < 0 || header.Size > 2<<30 || total+header.Size > 2<<30 {
			return fmt.Errorf("invalid lifecycle archive")
		}
		total += header.Size
		path := filepath.Join(root, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err = ensureDirectory(path); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = ensureDirectory(filepath.Dir(path)); err != nil {
				return err
			}
			mode := os.FileMode(0600)
			if header.Mode&0111 != 0 {
				mode = 0700
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("lifecycle archive links are not allowed")
		}
	}
	for _, name := range []string{"python/bin/python3.12", "engine.py", "manifest.json"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("incomplete lifecycle archive")
		}
	}
	return nil
}
