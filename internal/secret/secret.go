package secret

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"ariadesk/internal/config"

	"golang.org/x/sys/windows"
)

var (
	crypt32 = windows.NewLazySystemDLL("crypt32.dll")

	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func FilePath() string {
	return filepath.Join(config.DataDir(), "token.pass")
}

func protect(data []byte) ([]byte, error) {
	var in, out dataBlob
	in.cbData = uint32(len(data))
	if len(data) > 0 {
		in.pbData = &data[0]
	}
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	return buf, nil
}

func unprotect(data []byte) ([]byte, error) {
	var in, out dataBlob
	in.cbData = uint32(len(data))
	if len(data) > 0 {
		in.pbData = &data[0]
	}
	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	return buf, nil
}

func Generate() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Save(token string) error {
	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return err
	}
	enc, err := protect([]byte(token))
	if err != nil {
		return err
	}
	return os.WriteFile(FilePath(), enc, 0o600)
}

func Load() (string, error) {
	data, err := os.ReadFile(FilePath())
	if err != nil {
		return "", err
	}
	dec, err := unprotect(data)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}

func LoadOrCreate() (string, error) {
	token, err := Load()
	if err == nil && strings.TrimSpace(token) != "" {
		return token, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	token, err = Generate()
	if err != nil {
		return "", err
	}
	if err := Save(token); err != nil {
		return "", err
	}
	return token, nil
}
