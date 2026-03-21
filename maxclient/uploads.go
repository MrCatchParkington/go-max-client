package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// UploadPhoto uploads a photo and returns an Attachment for use in SendMessage.
func (c *Client) UploadPhoto(ctx context.Context, filename string, data io.Reader) (*Attachment, error) {
	// Get upload URL first (server expects this order)
	resp, err := c.InvokeMethod(ctx, OpcodePhotoUploadURL, map[string]any{"count": 1})
	if err != nil {
		return nil, fmt.Errorf("get photo upload URL: %w", err)
	}

	var urlPayload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(resp.Payload, &urlPayload); err != nil {
		return nil, fmt.Errorf("parse upload URL: %w", err)
	}

	// Upload via HTTP
	httpResp, err := c.uploadHTTP(ctx, urlPayload.URL, filename, data)
	if err != nil {
		return nil, fmt.Errorf("upload photo: %w", err)
	}
	defer httpResp.Body.Close()

	var result struct {
		Photos map[string]struct {
			Token string `json:"token"`
		} `json:"photos"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}

	var token string
	for _, photo := range result.Photos {
		token = photo.Token
		break
	}
	if token == "" {
		return nil, fmt.Errorf("upload photo: no token in response")
	}

	return &Attachment{Type: "PHOTO", PhotoToken: token}, nil
}

// UploadVideo uploads a video and waits for server-side processing completion (opcode 136).
func (c *Client) UploadVideo(ctx context.Context, filename string, data io.Reader) (*Attachment, error) {
	// Get upload URL first (server expects this order)
	resp, err := c.InvokeMethod(ctx, OpcodeVideoUploadURL, map[string]any{"count": 1})
	if err != nil {
		return nil, fmt.Errorf("get video upload URL: %w", err)
	}

	var urlPayload struct {
		Info []struct {
			URL     string      `json:"url"`
			VideoID json.Number `json:"videoId"`
			Token   string      `json:"token"`
		} `json:"info"`
	}
	if err := json.Unmarshal(resp.Payload, &urlPayload); err != nil {
		return nil, fmt.Errorf("parse video upload URL: %w", err)
	}
	if len(urlPayload.Info) == 0 {
		return nil, fmt.Errorf("parse video upload URL: no upload info in response")
	}

	info := urlPayload.Info[0]
	videoID := info.VideoID.String()

	// Register pending completion before upload
	done := make(chan error, 1)
	c.videoPending.Store(videoID, done)
	defer c.videoPending.Delete(videoID)

	// Upload via HTTP
	httpResp, err := c.uploadHTTP(ctx, info.URL, filename, data)
	if err != nil {
		return nil, fmt.Errorf("upload video: %w", err)
	}
	httpResp.Body.Close()

	// Wait for opcode 136 completion or disconnect
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("upload video: %w", err)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &Attachment{Type: "VIDEO", VideoID: json.Number(videoID), Token: info.Token}, nil
}

// UploadFile uploads a file and waits for server-side processing completion (opcode 136).
func (c *Client) UploadFile(ctx context.Context, filename string, data io.Reader) (*Attachment, error) {
	// Get upload URL first (server expects this order)
	resp, err := c.InvokeMethod(ctx, OpcodeFileUploadURL, map[string]any{"count": 1})
	if err != nil {
		return nil, fmt.Errorf("get file upload URL: %w", err)
	}

	var urlPayload struct {
		Info []struct {
			URL    string      `json:"url"`
			FileID json.Number `json:"fileId"`
		} `json:"info"`
	}
	if err := json.Unmarshal(resp.Payload, &urlPayload); err != nil {
		return nil, fmt.Errorf("parse file upload URL: %w", err)
	}
	if len(urlPayload.Info) == 0 {
		return nil, fmt.Errorf("parse file upload URL: no upload info in response")
	}

	info := urlPayload.Info[0]
	fileID := info.FileID.String()

	// Register pending completion before upload
	done := make(chan error, 1)
	c.filePending.Store(fileID, done)
	defer c.filePending.Delete(fileID)

	// Upload via HTTP
	httpResp, err := c.uploadHTTP(ctx, info.URL, filename, data)
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}
	httpResp.Body.Close()

	// Wait for opcode 136 completion or disconnect
	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("upload file: %w", err)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &Attachment{Type: "FILE", FileID: json.Number(fileID)}, nil
}

// uploadHTTP performs a multipart file upload to the given URL.
func (c *Client) uploadHTTP(ctx context.Context, url, filename string, data io.Reader) (*http.Response, error) {
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)

	go func() {
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, data); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(w.Close())
	}()

	req, err := http.NewRequestWithContext(ctx, "POST", url, pr)
	if err != nil {
		pr.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Origin", DefaultOrigin)
	req.Header.Set("Referer", DefaultOrigin+"/")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("upload HTTP %d", resp.StatusCode)
	}
	return resp, nil
}
