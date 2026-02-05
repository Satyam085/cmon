// Package errors provides custom error types for the CMON application.
//
// This package defines domain-specific errors that help with error handling
// and recovery throughout the application. Each error type provides context
// about what went wrong and can be used for specific recovery strategies.
package errors

import "fmt"

// SessionExpiredError indicates that the user session has expired and needs re-authentication.
//
// This error is returned when:
//   - The login form is detected on a page that should show the dashboard
//   - Session cookies have expired
//   - The user has been logged out by the server
//
// Recovery strategy: Re-login with credentials
type SessionExpiredError struct {
	Message string
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("session expired: %s", e.Message)
}

// NewSessionExpiredError creates a new session expired error with context
func NewSessionExpiredError(msg string) *SessionExpiredError {
	return &SessionExpiredError{Message: msg}
}

// LoginFailedError indicates that a login attempt failed.
//
// This error is returned when:
//   - Navigation to login page fails
//   - Captcha solving fails
//   - Form submission fails
//   - Credentials are rejected
//
// Recovery strategy: Retry with exponential backoff, then restart browser
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

// Unwrap returns the wrapped error for error chain inspection
func (e *LoginFailedError) Unwrap() error {
	return e.Err
}

// NewLoginFailedError creates a new login failed error with context
func NewLoginFailedError(msg string, err error) *LoginFailedError {
	return &LoginFailedError{Message: msg, Err: err}
}

// FetchError wraps fetch-related errors that occur during complaint retrieval.
//
// This error is returned when:
//   - Navigation to dashboard fails
//   - Table loading times out
//   - Page scraping fails
//   - API calls fail
//
// Recovery strategy: Retry, check for session expiry, restart browser if needed
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

// Unwrap returns the wrapped error for error chain inspection
func (e *FetchError) Unwrap() error {
	return e.Err
}

// NewFetchError creates a new fetch error with context
func NewFetchError(msg string, err error) *FetchError {
	return &FetchError{Message: msg, Err: err}
}

// IsLoginFailed checks if the error is a login failure error
func IsLoginFailed(err error) bool {
	_, ok := err.(*LoginFailedError)
	return ok
}

// IsSessionExpired checks if the error is a session expired error
func IsSessionExpired(err error) bool {
	_, ok := err.(*SessionExpiredError)
	return ok
}
