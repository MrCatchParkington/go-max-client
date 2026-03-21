package maxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
)

func TestUploadPhoto(t *testing.T) {
	// Mock HTTP upload server
	var uploadedData []byte
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, _, _ := r.FormFile("file")
		uploadedData, _ = io.ReadAll(f)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"photos": map[string]any{
				"0": map[string]any{"token": "photo-tok-123"},
			},
		})
	}))
	defer httpSrv.Close()

	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodePhotoUploadURL: respondOK(`{"url":"` + httpSrv.URL + `"}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	data := []byte("fake-image-data")
	att, err := c.UploadPhoto(ctx, "test.jpg", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if att.Type != "PHOTO" {
		t.Errorf("type = %q, want PHOTO", att.Type)
	}
	if att.PhotoToken != "photo-tok-123" {
		t.Errorf("token = %q, want photo-tok-123", att.PhotoToken)
	}
	if !bytes.Equal(uploadedData, data) {
		t.Error("uploaded data mismatch")
	}
}

func TestUploadVideo(t *testing.T) {
	// uploadDone is signalled by the HTTP handler after the upload completes,
	// so the mock WS server sends opcode 136 only after the client has
	// registered the videoPending channel.
	uploadDone := make(chan struct{})

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		close(uploadDone)
	}))
	defer httpSrv.Close()

	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeVideoUploadURL: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			// Respond with upload URL
			// Real server returns videoId as a number
			if err := respondOK(`{"info":[{"url":"` + httpSrv.URL + `","videoId":12345,"token":"vid-tok"}]}`)(ctx, conn, pkt); err != nil {
				return err
			}
			// Wait for the HTTP upload to complete before sending opcode 136,
			// ensuring the client has stored the videoPending entry.
			go func() {
				<-uploadDone
				completion, _ := json.Marshal(Packet{
					Ver: RPCVersion, Seq: 0, Opcode: OpcodeUploadComplete,
					Payload: json.RawMessage(`{"videoId":12345}`),
				})
				conn.Write(ctx, websocket.MessageText, completion)
			}()
			return nil
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	att, err := c.UploadVideo(ctx, "test.mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if att.Type != "VIDEO" {
		t.Errorf("type = %q, want VIDEO", att.Type)
	}
	if string(att.VideoID) != "12345" {
		t.Errorf("videoId = %q, want 12345", att.VideoID)
	}
}

func TestUploadFile(t *testing.T) {
	uploadDone := make(chan struct{})
	fileHTTPSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		close(uploadDone)
	}))
	defer fileHTTPSrv.Close()

	srv, wsURL := mockWSServer(map[int]mockHandler{
		// Real server returns fileId as a number
		OpcodeFileUploadURL: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			if err := respondOK(`{"info":[{"url":"` + fileHTTPSrv.URL + `","fileId":12345}]}`)(ctx, conn, pkt); err != nil {
				return err
			}
			// Wait for HTTP upload to complete before sending opcode 136
			go func() {
				<-uploadDone
				completion, _ := json.Marshal(Packet{
					Ver: RPCVersion, Seq: 0, Opcode: OpcodeUploadComplete,
					Payload: json.RawMessage(`{"fileId":12345}`),
				})
				conn.Write(ctx, websocket.MessageText, completion)
			}()
			return nil
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	att, err := c.UploadFile(ctx, "doc.pdf", bytes.NewReader([]byte("pdf-data")))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if att.Type != "FILE" {
		t.Errorf("type = %q, want FILE", att.Type)
	}
	if string(att.FileID) != "12345" {
		t.Errorf("fileId = %q, want 12345", att.FileID)
	}
}
