// Package toq provides a client for the local toq daemon API.
//
// The daemon handles all protocol complexity (crypto, TLS, handshake,
// connections). This SDK provides a clean interface for agent code.
//
// Usage:
//
//	client := toq.Connect("")
//	resp, _ := client.Send("toq://peer.com/agent", "hello", nil)
//
//	msgs, _ := client.Messages()
//	for msg := range msgs {
//	    msg.Reply("got it")
//	}
package toq

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultURL = "http://127.0.0.1:9010"
	URLEnv     = "TOQ_API_URL"
)

// ToqError is returned when the SDK cannot communicate with the daemon.
type ToqError struct {
	Message string
}

func (e *ToqError) Error() string { return e.Message }

// Message represents an incoming message from a remote agent.
type Message struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	From        string      `json:"from"`
	Body        interface{} `json:"body,omitempty"`
	ThreadID    string      `json:"thread_id,omitempty"`
	ReplyTo     string      `json:"reply_to,omitempty"`
	ContentType string      `json:"content_type,omitempty"`
	Timestamp   string      `json:"timestamp"`
	client      *Client
}

// Reply sends a reply to this message.
func (m *Message) Reply(text string) (map[string]interface{}, error) {
	return m.client.Send(m.From, text, &SendOptions{
		ThreadID: m.ThreadID,
		ReplyTo:  m.ID,
	})
}

// SendOptions are optional parameters for Send.
type SendOptions struct {
	ThreadID    string
	ReplyTo     string
	CloseThread bool
	Wait        *bool
	Timeout     int
}

// Connect creates a new Client to the local toq daemon.
//
// Resolution order:
//  1. Explicit baseURL parameter
//  2. TOQ_API_URL environment variable
//  3. .toq/state.json in current directory (workspace mode)
//  4. Default http://127.0.0.1:9010
func Connect(baseURL string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv(URLEnv)
	}
	if baseURL == "" {
		if data, err := os.ReadFile(filepath.Join(".toq", "state.json")); err == nil {
			var state map[string]interface{}
			if json.Unmarshal(data, &state) == nil {
				if port, ok := state["api_port"].(float64); ok && port > 0 {
					baseURL = fmt.Sprintf("http://127.0.0.1:%d", int(port))
				}
			}
		}
	}
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		url:  strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// Client is a client to the local toq daemon API.
type Client struct {
	url  string
	http *http.Client
}

func (c *Client) request(method, path string, body interface{}) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.url+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &ToqError{Message: "toq daemon is not running. Run 'toq up' first."}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &ToqError{Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(b))}
	}
	return resp, nil
}

func (c *Client) jsonRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	resp, err := c.request(method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

// ── Messages ────────────────────────────────────────────

// Send sends a message to a remote agent.
func (c *Client) Send(to, text string, opts *SendOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"to": to, "body": map[string]string{"text": text}}
	if opts != nil {
		if opts.ThreadID != "" {
			body["thread_id"] = opts.ThreadID
		}
		if opts.ReplyTo != "" {
			body["reply_to"] = opts.ReplyTo
		}
		if opts.CloseThread {
			body["close_thread"] = true
		}
	}
	wait := true
	timeout := 30
	if opts != nil {
		if opts.Wait != nil {
			wait = *opts.Wait
		}
		if opts.Timeout != 0 {
			timeout = opts.Timeout
		}
	}
	path := fmt.Sprintf("/v1/messages?wait=%t&timeout=%d", wait, timeout)
	return c.jsonRequest("POST", path, body)
}

// Messages returns a channel of incoming messages via SSE.
func (c *Client) Messages() (<-chan Message, error) {
	return c.MessagesFiltered("", "")
}

// MessagesFiltered returns a channel of incoming messages via SSE with optional filters.
func (c *Client) MessagesFiltered(from, msgType string) (<-chan Message, error) {
	path := "/v1/messages"
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if msgType != "" {
		q.Set("type", msgType)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	resp, err := c.request("GET", path, nil)
	if err != nil {
		return nil, err
	}
	ch := make(chan Message)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(line[6:]), &data); err != nil {
				continue
			}
			msg := Message{
				ID:          str(data, "id"),
				Type:        str(data, "type"),
				From:        str(data, "from"),
				Body:        data["body"],
				ThreadID:    str(data, "thread_id"),
				ReplyTo:     str(data, "reply_to"),
				ContentType: str(data, "content_type"),
				Timestamp:   str(data, "timestamp"),
				client:      c,
			}
			ch <- msg
		}
	}()
	return ch, nil
}

// SendMulti sends the same message to multiple agents, each on its own thread.
func (c *Client) SendMulti(to []string, text string, opts *SendOptions) (map[string]interface{}, error) {
	body := map[string]interface{}{"to": to, "body": map[string]string{"text": text}}
	if opts != nil {
		if opts.ThreadID != "" {
			body["thread_id"] = opts.ThreadID
		}
		if opts.CloseThread {
			body["close_thread"] = true
		}
	}
	wait := true
	timeout := 30
	if opts != nil {
		if opts.Wait != nil {
			wait = *opts.Wait
		}
		if opts.Timeout != 0 {
			timeout = opts.Timeout
		}
	}
	path := fmt.Sprintf("/v1/messages?wait=%t&timeout=%d", wait, timeout)
	return c.jsonRequest("POST", path, body)
}

// StreamStart opens a streaming connection to a remote agent.
func (c *Client) StreamStart(to string, threadID string) (map[string]interface{}, error) {
	body := map[string]interface{}{"to": to}
	if threadID != "" {
		body["thread_id"] = threadID
	}
	return c.jsonRequest("POST", "/v1/stream/start", body)
}

// StreamChunk sends a text chunk on an open stream.
func (c *Client) StreamChunk(streamID, text string) (map[string]interface{}, error) {
	return c.jsonRequest("POST", "/v1/stream/chunk", map[string]string{"stream_id": streamID, "text": text})
}

// StreamEnd ends a stream, optionally closing the thread.
func (c *Client) StreamEnd(streamID string, closeThread bool) (map[string]interface{}, error) {
	body := map[string]interface{}{"stream_id": streamID}
	if closeThread {
		body["close_thread"] = true
	}
	return c.jsonRequest("POST", "/v1/stream/end", body)
}

// ── Threads ─────────────────────────────────────────────

// GetThread returns messages in a thread.
func (c *Client) GetThread(threadID string) (map[string]interface{}, error) {
	return c.jsonRequest("GET", "/v1/threads/"+threadID, nil)
}

// ── Peers ───────────────────────────────────────────────

func (c *Client) Peers() ([]interface{}, error) { return c.listField("GET", "/v1/peers", "peers") }

// BlockByKey blocks an agent by public key.
func (c *Client) BlockByKey(key string) error {
	return c.do2("POST", "/v1/block", map[string]string{"key": key})
}

// BlockByAddress blocks agents matching an address pattern.
func (c *Client) BlockByAddress(from string) error {
	return c.do2("POST", "/v1/block", map[string]string{"from": from})
}

// Block blocks an agent by public key (backward compat).
func (c *Client) Block(publicKey string) error { return c.BlockByKey(publicKey) }

// UnblockByKey removes a key-based block rule.
func (c *Client) UnblockByKey(key string) error {
	return c.doWithBody("DELETE", "/v1/block", map[string]string{"key": key})
}

// UnblockByAddress removes an address-based block rule.
func (c *Client) UnblockByAddress(from string) error {
	return c.doWithBody("DELETE", "/v1/block", map[string]string{"from": from})
}

// Unblock removes a key-based block rule (backward compat).
func (c *Client) Unblock(publicKey string) error { return c.UnblockByKey(publicKey) }

// ── Approvals ───────────────────────────────────────────

func (c *Client) Approvals() ([]interface{}, error) {
	return c.listField("GET", "/v1/approvals", "approvals")
}

// Approve approves a pending request by ID (backward compat).
func (c *Client) Approve(id string) error {
	return c.do2("POST", "/v1/approvals/"+url.PathEscape(id), map[string]string{"decision": "approve"})
}

// ApproveByKey adds a key-based approve rule.
func (c *Client) ApproveByKey(key string) error {
	return c.do2("POST", "/v1/approve", map[string]string{"key": key})
}

// ApproveByAddress adds an address-based approve rule.
func (c *Client) ApproveByAddress(from string) error {
	return c.do2("POST", "/v1/approve", map[string]string{"from": from})
}

func (c *Client) Deny(id string) error {
	return c.do2("POST", "/v1/approvals/"+url.PathEscape(id), map[string]string{"decision": "deny"})
}

// Revoke removes an approve rule by pending ID (backward compat).
func (c *Client) Revoke(id string) error {
	return c.do("POST", "/v1/approvals/"+url.PathEscape(id)+"/revoke")
}

// RevokeByKey removes a key-based approve rule.
func (c *Client) RevokeByKey(key string) error {
	return c.do2("POST", "/v1/revoke", map[string]string{"key": key})
}

// RevokeByAddress removes an address-based approve rule.
func (c *Client) RevokeByAddress(from string) error {
	return c.do2("POST", "/v1/revoke", map[string]string{"from": from})
}

// ── Permissions ─────────────────────────────────────────

func (c *Client) Permissions() (map[string]interface{}, error) {
	return c.jsonResult("GET", "/v1/permissions")
}

// PingResult holds the response from a ping request.
type PingResult struct {
	AgentName string `json:"agent_name"`
	Address   string `json:"address"`
	PublicKey string `json:"public_key"`
	Reachable bool   `json:"reachable"`
}

func (c *Client) Ping(address string) (*PingResult, error) {
	body, err := json.Marshal(map[string]string{"address": address})
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Post(c.url+"/v1/ping", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result PingResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ── History ─────────────────────────────────────────────

type HistoryOptions struct {
	Limit int
	From  string
	Since string
}

func (c *Client) History(opts HistoryOptions) ([]interface{}, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.From != "" {
		q.Set("from", opts.From)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	path := "/v1/messages/history"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.listField("GET", path, "messages")
}

// ── Discovery ───────────────────────────────────────────

func (c *Client) Discover(host string) ([]interface{}, error) {
	return c.listField("GET", "/v1/discover?host="+url.QueryEscape(host), "agents")
}

func (c *Client) DiscoverLocal() ([]interface{}, error) {
	return c.listField("GET", "/v1/discover/local", "agents")
}

// ── Connections ─────────────────────────────────────────

func (c *Client) Connections() ([]interface{}, error) {
	return c.listField("GET", "/v1/connections", "connections")
}

// ── Daemon ──────────────────────────────────────────────

func (c *Client) Health() (string, error) {
	resp, err := c.request("GET", "/v1/health", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) Status() (map[string]interface{}, error)      { return c.jsonRequest("GET", "/v1/status", nil) }
func (c *Client) Shutdown(graceful bool) error                  { return c.do2("POST", "/v1/daemon/shutdown", map[string]bool{"graceful": graceful}) }
func (c *Client) Logs() ([]interface{}, error)                  { return c.listField("GET", "/v1/logs", "entries") }

// FollowLogs streams log entries in real time via SSE.
func (c *Client) FollowLogs() (<-chan map[string]interface{}, error) {
	resp, err := c.request("GET", "/v1/logs?follow=true", nil)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]interface{})
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(line[6:]), &entry); err != nil {
				continue
			}
			ch <- entry
		}
	}()
	return ch, nil
}
func (c *Client) ClearLogs() error                              { return c.do("DELETE", "/v1/logs") }
func (c *Client) Diagnostics() (map[string]interface{}, error)  { return c.jsonRequest("GET", "/v1/diagnostics", nil) }
func (c *Client) CheckUpgrade() (map[string]interface{}, error) { return c.jsonRequest("GET", "/v1/upgrade/check", nil) }

// ── Keys ────────────────────────────────────────────────

func (c *Client) RotateKeys() (map[string]interface{}, error) { return c.jsonRequest("POST", "/v1/keys/rotate", nil) }

// ── Backup ──────────────────────────────────────────────

func (c *Client) ExportBackup(passphrase string) (string, error) {
	result, err := c.jsonRequest("POST", "/v1/backup/export", map[string]string{"passphrase": passphrase})
	if err != nil {
		return "", err
	}
	return str(result, "data"), nil
}

func (c *Client) ImportBackup(passphrase, data string) error {
	return c.do2("POST", "/v1/backup/import", map[string]string{"passphrase": passphrase, "data": data})
}

// ── Config ──────────────────────────────────────────────

func (c *Client) Config() (map[string]interface{}, error) {
	result, err := c.jsonRequest("GET", "/v1/config", nil)
	if err != nil {
		return nil, err
	}
	if cfg, ok := result["config"].(map[string]interface{}); ok {
		return cfg, nil
	}
	return result, nil
}

func (c *Client) UpdateConfig(updates map[string]interface{}) (map[string]interface{}, error) {
	result, err := c.jsonRequest("PATCH", "/v1/config", updates)
	if err != nil {
		return nil, err
	}
	if cfg, ok := result["config"].(map[string]interface{}); ok {
		return cfg, nil
	}
	return result, nil
}

// ── Agent Card ──────────────────────────────────────────

func (c *Client) Card() (map[string]interface{}, error) { return c.jsonRequest("GET", "/v1/card", nil) }

// ── Handlers ────────────────────────────────────────────

func (c *Client) Handlers() ([]interface{}, error) {
	return c.listField("GET", "/v1/handlers", "handlers")
}

func (c *Client) AddHandler(name, command string, filterFrom, filterKey, filterType []string) (map[string]interface{}, error) {
	body := map[string]interface{}{"name": name, "command": command}
	if len(filterFrom) > 0 {
		body["filter_from"] = filterFrom
	}
	if len(filterKey) > 0 {
		body["filter_key"] = filterKey
	}
	if len(filterType) > 0 {
		body["filter_type"] = filterType
	}
	return c.jsonRequest("POST", "/v1/handlers", body)
}

func (c *Client) RemoveHandler(name string) (map[string]interface{}, error) {
	return c.jsonRequest("DELETE", "/v1/handlers/"+url.PathEscape(name), nil)
}

func (c *Client) UpdateHandler(name string, updates map[string]interface{}) (map[string]interface{}, error) {
	return c.jsonRequest("PUT", "/v1/handlers/"+url.PathEscape(name), updates)
}

func (c *Client) StopHandler(name string, pid *int) (map[string]interface{}, error) {
	body := map[string]interface{}{"name": name}
	if pid != nil {
		body["pid"] = *pid
	}
	return c.jsonRequest("POST", "/v1/handlers/stop", body)
}

// ── Helpers ─────────────────────────────────────────────

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (c *Client) do(method, path string) error {
	resp, err := c.request(method, path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) do2(method, path string, body interface{}) error {
	resp, err := c.request(method, path, body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) doWithBody(method, path string, body interface{}) error {
	return c.do2(method, path, body)
}

func (c *Client) jsonResult(method, path string) (map[string]interface{}, error) {
	return c.jsonRequest(method, path, nil)
}

func (c *Client) listField(method, path, field string) ([]interface{}, error) {
	result, err := c.jsonRequest(method, path, nil)
	if err != nil {
		return nil, err
	}
	if items, ok := result[field].([]interface{}); ok {
		return items, nil
	}
	return nil, nil
}

// Bool returns a pointer to a bool value. Useful for SendOptions.Wait.
func Bool(v bool) *bool { return &v }
