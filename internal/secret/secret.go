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

// ExtFilePath 浏览器插件配对 token 的存储路径，与 aria2 rpc-secret 隔离
func ExtFilePath() string {
	return filepath.Join(config.DataDir(), "ext.pass")
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
	return saveTo(FilePath(), token)
}

func Load() (string, error) {
	return loadFrom(FilePath())
}

func LoadOrCreate() (string, error) {
	return loadOrCreate(FilePath())
}

// SaveExt / LoadExt / LoadOrCreateExt 为浏览器插件配对 token 的存取
func SaveExt(token string) error {
	return saveTo(ExtFilePath(), token)
}

func LoadExt() (string, error) {
	return loadFrom(ExtFilePath())
}

func LoadOrCreateExt() (string, error) {
	return loadOrCreate(ExtFilePath())
}

func saveTo(path, token string) error {
	if err := os.MkdirAll(config.DataDir(), 0o755); err != nil {
		return err
	}
	enc, err := protect([]byte(token))
	if err != nil {
		return err
	}
	return os.WriteFile(path, enc, 0o600)
}

func loadFrom(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dec, err := unprotect(data)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}

func loadOrCreate(path string) (string, error) {
	token, err := loadFrom(path)
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
	if err := saveTo(path, token); err != nil {
		return "", err
	}
	return token, nil
}
