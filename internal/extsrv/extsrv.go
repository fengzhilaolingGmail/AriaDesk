// Package extsrv 为浏览器插件提供本地 HTTP 推送服务：
// 在 127.0.0.1:20000~20005 内协商监听端口，鉴权后把下载请求转发给 aria2。
package extsrv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	PortMin    = 20000
	PortMax    = 20005
	APIVersion = 1

	maxBodyBytes = 10 << 20 // 10 MB，覆盖 base64 种子
)

// ErrAria2NotRunning 表示收到推送时 aria2 不可用且自动拉起失败，
// 插件侧可据此提示用户启动下载服务。
var ErrAria2NotRunning = errors.New("aria2 未运行")

type DownloadRequest struct {
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	Referer   string `json:"referer"`
	Cookies   string `json:"cookies"`
	UserAgent string `json:"userAgent"`
}

type TorrentRequest struct {
	Name       string `json:"name"`
	DataBase64 string `json:"dataBase64"`
}

// Handler 由 App 实现，负责把校验过的请求落到 aria2。
type Handler interface {
	AddDownload(req DownloadRequest) (gid string, err error)
	AddTorrent(name string, data []byte) (gid string, err error)
}

type Server struct {
	appVersion string
	tokenFunc  func() (string, error)
	handler    Handler

	mu      sync.Mutex
	httpSrv *http.Server
	port    int
}

// tokenFunc 每次请求动态取 token，重新生成后无需重启服务即生效。
func New(appVersion string, tokenFunc func() (string, error), handler Handler) *Server {
	return &Server{appVersion: appVersion, tokenFunc: tokenFunc, handler: handler}
}

// Start 在 PortMin~PortMax 内绑定第一个空闲端口并启动服务。
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv != nil {
		return nil
	}
	var ln net.Listener
	var port int
	for p := PortMin; p <= PortMax; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			ln = l
			port = p
			break
		}
	}
	if ln == nil {
		return fmt.Errorf("端口 %d~%d 均被占用", PortMin, PortMax)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", s.handlePing)
	mux.HandleFunc("POST /api/download", s.withAuth(s.handleDownload))
	mux.HandleFunc("POST /api/torrent", s.withAuth(s.handleTorrent))

	s.httpSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	s.port = port
	go func() { _ = s.httpSrv.Serve(ln) }()
	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = s.httpSrv.Shutdown(ctx)
	cancel()
	s.httpSrv = nil
	s.port = 0
}

func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpSrv != nil
}

func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

type apiResponse struct {
	GID   string `json:"gid,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	resp := apiResponse{Error: err.Error()}
	if errors.Is(err, ErrAria2NotRunning) {
		resp.Code = "ARIA2_NOT_RUNNING"
	}
	writeJSON(w, status, resp)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	// ping 无需鉴权，只暴露应用名与版本，供插件扫描锁定端口
	writeJSON(w, http.StatusOK, map[string]any{
		"app":     "AriaDesk",
		"version": s.appVersion,
		"api":     APIVersion,
	})
}

// withAuth 校验配对 token。自定义头使恶意网页的跨域请求必先过 CORS 预检，
// 本服务不响应预检，网页无法伪造；插件从 service worker 发请求不受 CORS 限制。
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, err := s.tokenFunc()
		if err != nil || token == "" {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("读取配对 token 失败"))
			return
		}
		if r.Header.Get("X-AriaDesk-Token") != token {
			writeJSON(w, http.StatusUnauthorized, apiResponse{Error: "配对 token 无效", Code: "UNAUTHORIZED"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	var req DownloadRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if err := validateURL(req.URL); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.Filename = sanitizeFilename(req.Filename)
	gid, err := s.handler.AddDownload(req)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{GID: gid})
}

func (s *Server) handleTorrent(w http.ResponseWriter, r *http.Request) {
	var req TorrentRequest
	if err := decodeBody(w, r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil || len(data) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("种子数据无效"))
		return
	}
	gid, err := s.handler.AddTorrent(sanitizeFilename(req.Name), data)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{GID: gid})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("请求格式错误: %v", err)
	}
	return nil
}

func validateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("下载地址不能为空")
	}
	if strings.HasPrefix(raw, "magnet:") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("仅支持 http/https/magnet 链接")
	}
	return nil
}

// sanitizeFilename 去掉路径成分，防止 out 选项被注入目录穿越
func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = filepath.Base(name)
	if name == "." || name == ".." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

func statusFor(err error) int {
	if errors.Is(err, ErrAria2NotRunning) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}
