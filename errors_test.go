package main

import (
	"testing"
)

func TestSessionExpiredError(t *testing.T) {
	err := NewSessionExpiredError("test message")
	expected := "session expired: test message"
	
	if err.Error() != expected {
		t.Errorf("expected %q but got %q", expected, err.Error())
	}
}

func TestLoginFailedError(t *testing.T) {
	baseErr := NewSessionExpiredError("base error")
	err := NewLoginFailedError("login failed", baseErr)
	
	if err.Message != "login failed" {
		t.Errorf("expected message 'login failed' but got %q", err.Message)
	}
	
	if err.Err == nil {
		t.Error("expected wrapped error but got nil")
	}
}

func TestFetchError(t *testing.T) {
	baseErr := NewSessionExpiredError("base error")
	err := NewFetchError("fetch failed", baseErr)
	
	if err.Message != "fetch failed" {
		t.Errorf("expected message 'fetch failed' but got %q", err.Message)
	}
	
	errorString := err.Error()
	if errorString == "" {
		t.Error("expected non-empty error string")
	}
}

func TestIsLoginFailed(t *testing.T) {
	loginErr := NewLoginFailedError("test", nil)
	if !IsLoginFailed(loginErr) {
		t.Error("expected IsLoginFailed to return true for LoginFailedError")
	}
	
	otherErr := NewSessionExpiredError("test")
	if IsLoginFailed(otherErr) {
		t.Error("expected IsLoginFailed to return false for non-LoginFailedError")
	}
}
