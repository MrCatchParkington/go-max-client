package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type callsAPI struct {
	baseURL    string
	httpClient *http.Client
}

func newCallsAPI(httpClient *http.Client) *callsAPI {
	return &callsAPI{
		baseURL:    CallsAPIURL,
		httpClient: httpClient,
	}
}

func (a *callsAPI) post(ctx context.Context, params url.Values, result any) error {
	params.Set("format", "JSON")
	params.Set("application_key", CallsAppKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", DefaultOrigin)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB max
	if err != nil {
		return err
	}
	slog.Debug("calls API response", "method", params.Get("method"), "status", resp.StatusCode, "bodyLen", len(body))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("calls API: status %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, result)
}

func (a *callsAPI) login(ctx context.Context, callToken string) (*callsLoginResponse, error) {
	deviceID, err := newUUID()
	if err != nil {
		return nil, err
	}
	sessionData := callsSessionData{
		Token:         callToken,
		ClientType:    CallsClientType,
		ClientVersion: CallsClientVersion,
		DeviceID:      deviceID,
		Version:       3,
	}
	sdJSON, err := json.Marshal(sessionData)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"method":       {"auth.anonymLogin"},
		"session_data": {string(sdJSON)},
	}
	var result callsLoginResponse
	if err := a.post(ctx, params, &result); err != nil {
		return nil, fmt.Errorf("calls login: %w", err)
	}
	if result.SessionKey == "" || result.ExternalUserID == "" {
		return nil, fmt.Errorf("calls login: missing session_key or external_user_id in response")
	}
	return &result, nil
}
