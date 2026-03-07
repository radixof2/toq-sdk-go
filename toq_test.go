package toq

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnectDefault(t *testing.T) {
	client := Connect("")
	if client.url != DefaultURL {
		t.Errorf("expected %s, got %s", DefaultURL, client.url)
	}
}

func TestConnectCustomURL(t *testing.T) {
	client := Connect("http://localhost:8080")
	if client.url != "http://localhost:8080" {
		t.Errorf("expected http://localhost:8080, got %s", client.url)
	}
}

func TestConnectEnvVar(t *testing.T) {
	t.Setenv(URLEnv, "http://custom:1234")
	client := Connect("")
	if client.url != "http://custom:1234" {
		t.Errorf("expected http://custom:1234, got %s", client.url)
	}
}

func TestConnectExplicitOverridesEnv(t *testing.T) {
	t.Setenv(URLEnv, "http://from-env:1234")
	client := Connect("http://explicit:5678")
	if client.url != "http://explicit:5678" {
		t.Errorf("expected http://explicit:5678, got %s", client.url)
	}
}

func TestDaemonNotRunning(t *testing.T) {
	client := Connect("http://127.0.0.1:19999")
	_, err := client.Status()
	if err == nil {
		t.Fatal("expected error")
	}
	toqErr, ok := err.(*ToqError)
	if !ok {
		t.Fatalf("expected ToqError, got %T", err)
	}
	if toqErr.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}


// --- Client methods with mock HTTP server ---

func mockServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestSend(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m1","status":"delivered","thread_id":"t1","timestamp":"now"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Send("toq://host/agent", "hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "delivered" {
		t.Errorf("expected delivered, got %v", result["status"])
	}
}

func TestPeers(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"peers":[{"public_key":"k1","address":"a1","status":"connected","last_seen":"now"}]}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 peer, got %d", len(result))
	}
}

func TestBlock(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(200)
	})
	defer srv.Close()

	client := Connect(srv.URL)
	err := client.Block("ed25519:abc")
	if err != nil {
		t.Fatal(err)
	}
}

func TestUnblock(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(200)
	})
	defer srv.Close()

	client := Connect(srv.URL)
	err := client.Unblock("ed25519:abc")
	if err != nil {
		t.Fatal(err)
	}
}

func TestApprovals(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"approvals":[{"id":"k1","public_key":"k1","address":"a1","requested_at":"now"}]}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Approvals()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 approval, got %d", len(result))
	}
}

func TestApprove(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(200)
	})
	defer srv.Close()

	client := Connect(srv.URL)
	err := client.Approve("k1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeny(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	defer srv.Close()

	client := Connect(srv.URL)
	err := client.Deny("k1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestHealth(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Health()
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Errorf("expected ok, got %s", result)
	}
}

func TestStatusWithMock(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"running","address":"toq://localhost/agent"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Status()
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "running" {
		t.Errorf("expected running, got %v", result["status"])
	}
}

func TestShutdown(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(200)
	})
	defer srv.Close()

	client := Connect(srv.URL)
	err := client.Shutdown(true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNon200ReturnsError(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"code":"invalid","message":"bad request"}}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	_, err := client.Send("toq://host/agent", "hi", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}


func TestSendCloseThread(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m1","status":"delivered","thread_id":"t1","timestamp":"now"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.Send("toq://host/agent", "goodbye", &SendOptions{CloseThread: true})
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "delivered" {
		t.Errorf("expected delivered, got %v", result["status"])
	}
}

func TestSendMulti(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"to":"toq://host/a","id":"m1","thread_id":"t1","status":"queued"},{"to":"toq://host/b","id":"m2","thread_id":"t2","status":"queued"}],"timestamp":"now"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.SendMulti([]string{"toq://host/a", "toq://host/b"}, "hello both", nil)
	if err != nil {
		t.Fatal(err)
	}
	results, ok := result["results"].([]interface{})
	if !ok || len(results) != 2 {
		t.Errorf("expected 2 results, got %v", result["results"])
	}
}

func TestStreamStart(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/stream/start" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stream_id":"s1","thread_id":"t1"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.StreamStart("toq://host/agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if result["stream_id"] != "s1" {
		t.Errorf("expected s1, got %v", result["stream_id"])
	}
}

func TestStreamChunk(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"chunk_id":"c1"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.StreamChunk("s1", "hello ")
	if err != nil {
		t.Fatal(err)
	}
	if result["chunk_id"] != "c1" {
		t.Errorf("expected c1, got %v", result["chunk_id"])
	}
}

func TestStreamEnd(t *testing.T) {
	srv := mockServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"chunk_id":"e1"}`))
	})
	defer srv.Close()

	client := Connect(srv.URL)
	result, err := client.StreamEnd("s1", true)
	if err != nil {
		t.Fatal(err)
	}
	if result["chunk_id"] != "e1" {
		t.Errorf("expected e1, got %v", result["chunk_id"])
	}
}
