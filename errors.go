package main

import "fmt"

// SessionExpiredError indicates that the user session has expired
type SessionExpiredError struct {
	Message string
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("session expired: %s", e.Message)
}

// NewSessionExpiredError creates a new session expired error
func NewSessionExpiredError(msg string) *SessionExpiredError {
	return &SessionExpiredError{Message: msg}
}

// LoginFailedError indicates that login attempt failed
type LoginFailedError struct {
	Message string
	Err     error
}

func (e *LoginFailedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("login failed: %s: %v", e.Message, e.Err)
	}
	return fmt.Sprintf("login failed: %s", e.Message)
}

// NewLoginFailedError creates a new login failed error
func NewLoginFailedError(msg string, err error) *LoginFailedError {
	return &LoginFailedError{Message: msg, Err: err}
}

// FetchError wraps fetch-related errors
type FetchError struct {
	Message string
	Err     error
}

func (e *FetchError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("fetch error: %s: %v", e.Message, e.Err)
	}
	return fmt.Sprintf("fetch error: %s", e.Message)
}

// NewFetchError creates a new fetch error
func NewFetchError(msg string, err error) *FetchError {
	return &FetchError{Message: msg, Err: err}
}

// IsLoginFailed checks if the error is a login failure error
func IsLoginFailed(err error) bool {
	_, ok := err.(*LoginFailedError)
	return ok
}

