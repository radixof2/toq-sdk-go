package toq

import (
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
