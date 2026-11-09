package copilotsettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-github/v93/github"
)

func apiError(target Target, err error) error {
	return fmt.Errorf("%s: %w", target, withResponseDetails(err))
}

func runnerUpdateError(target Target, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return fmt.Errorf("runner update for %s succeeded but its response could not be decoded: %w", target, err)
	}
	return apiError(target, err)
}

func withResponseDetails(err error) error {
	response := errorResponse(err)
	if response == nil || response.Body == nil {
		return err
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body = io.NopCloser(bytes.NewReader(body))
	if readErr != nil {
		return fmt.Errorf("%w (failed to read API response details: %v)", err, readErr)
	}
	if details := strings.TrimSpace(string(body)); details != "" {
		return fmt.Errorf("%w (API response body: %q)", err, details)
	}
	return err
}

func errorResponse(err error) *http.Response {
	var apiErr *github.ErrorResponse
	if errors.As(err, &apiErr) {
		return apiErr.Response
	}
	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		return rateErr.Response
	}
	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		return abuseErr.Response
	}
	var twoFactorErr *github.TwoFactorAuthError
	if errors.As(err, &twoFactorErr) {
		return twoFactorErr.Response
	}
	var redirectErr *github.RedirectionError
	if errors.As(err, &redirectErr) {
		return redirectErr.Response
	}
	return nil
}
