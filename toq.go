// Package toq provides a client for the local toq daemon API.
//
// The daemon handles all protocol complexity (crypto, TLS, handshake,
// connections). This SDK provides a clean interface for agent code.
//
// Usage:
//
//	client := toq.Connect("")
//	resp, _ := client.Send("toq://peer.com/agent", "hello")
//
//	for msg := range client.Messages() {
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
	ThreadID string
	ReplyTo  string
	Wait     *bool
	Timeout  int
}

// Connect creates a new Client to the local toq daemon.
func Connect(baseURL string) *Client {
	if baseURL == "" {
		baseURL = os.Getenv(URLEnv)
	}
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{
		url: strings.TrimRight(baseURL, "/"),
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
	resp, err := c.request("GET", "/v1/messages", nil)
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

// CancelMessage cancels a sent message.
func (c *Client) CancelMessage(messageID string) error {
	resp, err := c.request("POST", "/v1/messages/"+messageID+"/cancel", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// SendStreaming sends a message using streaming delivery.
func (c *Client) SendStreaming(to, text string) (map[string]interface{}, error) {
	body := map[string]interface{}{"to": to, "body": map[string]string{"text": text}}
	return c.jsonRequest("POST", "/v1/messages/stream", body)
}

// ── Threads ─────────────────────────────────────────────

// GetThread returns messages in a thread.
func (c *Client) GetThread(threadID string) (map[string]interface{}, error) {
	return c.jsonRequest("GET", "/v1/threads/"+threadID, nil)
}

// ── Peers ───────────────────────────────────────────────

func (c *Client) Peers() ([]interface{}, error)    { return c.listField("GET", "/v1/peers", "peers") }
func (c *Client) Block(publicKey string) error      { return c.do("POST", "/v1/peers/"+publicKey+"/block") }
func (c *Client) Unblock(publicKey string) error    { return c.do("DELETE", "/v1/peers/"+publicKey+"/block") }

// ── Approvals ───────────────────────────────────────────

func (c *Client) Approvals() ([]interface{}, error) { return c.listField("GET", "/v1/approvals", "approvals") }

func (c *Client) Approve(id string) error {
	return c.do2("POST", "/v1/approvals/"+id, map[string]string{"decision": "approve"})
}

func (c *Client) Deny(id string) error {
	return c.do2("POST", "/v1/approvals/"+id, map[string]string{"decision": "deny"})
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
