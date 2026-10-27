package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
	counter atomic.Int64
}

func NewClient(port int, token string) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port),
		token:   token,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) SetToken(token string) { c.token = token }

func (c *Client) SetPort(port int) {
	c.baseURL = fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("aria2 rpc error %d: %s", e.Code, e.Message)
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func (c *Client) call(ctx context.Context, method string, params []any, result any) error {
	if c.token != "" {
		params = append([]any{"token:" + c.token}, params...)
	}
	id := fmt.Sprintf("ariaDesk-%d", c.counter.Add(1))
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.Error != nil {
		return r.Error
	}
	if result != nil && len(r.Result) > 0 {
		if err := json.Unmarshal(r.Result, result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) GetVersion(ctx context.Context) (VersionInfo, error) {
	var v VersionInfo
	err := c.call(ctx, "aria2.getVersion", nil, &v)
	return v, err
}

func (c *Client) GetGlobalStat(ctx context.Context) (GlobalStat, error) {
	var s GlobalStat
	err := c.call(ctx, "aria2.getGlobalStat", nil, &s)
	return s, err
}

func (c *Client) TellActive(ctx context.Context) ([]Task, error) {
	var tasks []Task
	err := c.call(ctx, "aria2.tellActive", nil, &tasks)
	return tasks, err
}

func (c *Client) TellWaiting(ctx context.Context, offset, num int) ([]Task, error) {
	var tasks []Task
	err := c.call(ctx, "aria2.tellWaiting", []any{offset, num}, &tasks)
	return tasks, err
}

func (c *Client) TellStopped(ctx context.Context, offset, num int) ([]Task, error) {
	var tasks []Task
	err := c.call(ctx, "aria2.tellStopped", []any{offset, num}, &tasks)
	return tasks, err
}

func (c *Client) TellStatus(ctx context.Context, gid string, keys ...string) (Task, error) {
	var t Task
	var params []any
	if len(keys) > 0 {
		params = []any{gid, keys}
	} else {
		params = []any{gid}
	}
	err := c.call(ctx, "aria2.tellStatus", params, &t)
	return t, err
}

func (c *Client) AddURI(ctx context.Context, uri string, options map[string]string) (string, error) {
	var gid string
	params := []any{[]string{uri}}
	if len(options) > 0 {
		params = append(params, options)
	}
	err := c.call(ctx, "aria2.addUri", params, &gid)
	return gid, err
}

func (c *Client) AddTorrent(ctx context.Context, data []byte, options map[string]string) (string, error) {
	var gid string
	encoded := base64.StdEncoding.EncodeToString(data)
	params := []any{encoded}
	if len(options) > 0 {
		params = append(params, []string{}, options)
	}
	err := c.call(ctx, "aria2.addTorrent", params, &gid)
	return gid, err
}

func (c *Client) Pause(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.pause", []any{gid}, nil)
}

func (c *Client) Unpause(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.unpause", []any{gid}, nil)
}

func (c *Client) Remove(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.remove", []any{gid}, nil)
}

func (c *Client) ForceRemove(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.forceRemove", []any{gid}, nil)
}

func (c *Client) RemoveDownloadResult(ctx context.Context, gid string) error {
	return c.call(ctx, "aria2.removeDownloadResult", []any{gid}, nil)
}

func (c *Client) PauseAll(ctx context.Context) error {
	return c.call(ctx, "aria2.pauseAll", nil, nil)
}

func (c *Client) UnpauseAll(ctx context.Context) error {
	return c.call(ctx, "aria2.unpauseAll", nil, nil)
}

func (c *Client) PurgeDownloadResult(ctx context.Context) error {
	return c.call(ctx, "aria2.purgeDownloadResult", nil, nil)
}

func (c *Client) ChangeGlobalOption(ctx context.Context, options map[string]string) error {
	return c.call(ctx, "aria2.changeGlobalOption", []any{options}, nil)
}

func (c *Client) Shutdown(ctx context.Context) error {
	return c.call(ctx, "aria2.shutdown", nil, nil)
}
