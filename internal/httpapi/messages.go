package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zeyad/whatsapp-gateway/internal/message"
)

type sendResponse struct {
	Success   bool      `json:"success"`
	MessageID string    `json:"message_id"`
	Timestamp time.Time `json:"timestamp"`
}

func (s *Server) handleSendText(c *gin.Context) {
	inst := instanceFrom(c)
	var req struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := bindStrict(c, &req); err != nil {
		s.badRequest(c, err)
		return
	}
	res, err := s.messages.SendText(c.Request.Context(), inst.ID, req.To, req.Text)
	if err != nil {
		s.fail(c, err, "failed to send text message")
		return
	}
	respond(c, http.StatusOK, sendResponse{Success: true, MessageID: res.MessageID, Timestamp: res.Timestamp})
}

func (s *Server) handleSendImage(c *gin.Context) {
	inst := instanceFrom(c)
	if isMultipart(c) {
		upload, err := readMultipartUpload(c, s.messages.MaxUploadBytes())
		if err != nil {
			s.mediaError(c, err)
			return
		}
		res, err := s.messages.SendImageBytes(c.Request.Context(), inst.ID, upload.Fields["to"], upload.Fields["caption"], upload.Data)
		if err != nil {
			s.fail(c, err, "failed to send image message")
			return
		}
		respond(c, http.StatusOK, sendResponse{Success: true, MessageID: res.MessageID, Timestamp: res.Timestamp})
		return
	}

	var req struct {
		To      string `json:"to"`
		URL     string `json:"url"`
		Caption string `json:"caption"`
	}
	if err := bindStrict(c, &req); err != nil {
		s.badRequest(c, err)
		return
	}
	res, err := s.messages.SendImageURL(c.Request.Context(), inst.ID, req.To, req.Caption, req.URL)
	if err != nil {
		s.fail(c, err, "failed to send image message")
		return
	}
	respond(c, http.StatusOK, sendResponse{Success: true, MessageID: res.MessageID, Timestamp: res.Timestamp})
}

func (s *Server) handleSendDocument(c *gin.Context) {
	inst := instanceFrom(c)
	if isMultipart(c) {
		upload, err := readMultipartUpload(c, s.messages.MaxUploadBytes())
		if err != nil {
			s.mediaError(c, err)
			return
		}
		filename := upload.Fields["filename"]
		if filename == "" {
			filename = upload.Filename
		}
		res, err := s.messages.SendDocumentBytes(c.Request.Context(), inst.ID, upload.Fields["to"], upload.Fields["caption"],
			filename, upload.Fields["mime"], upload.Data)
		if err != nil {
			s.fail(c, err, "failed to send document message")
			return
		}
		respond(c, http.StatusOK, sendResponse{Success: true, MessageID: res.MessageID, Timestamp: res.Timestamp})
		return
	}

	var req struct {
		To       string `json:"to"`
		URL      string `json:"url"`
		Caption  string `json:"caption"`
		Filename string `json:"filename"`
		Mime     string `json:"mime"`
	}
	if err := bindStrict(c, &req); err != nil {
		s.badRequest(c, err)
		return
	}
	res, err := s.messages.SendDocumentURL(c.Request.Context(), inst.ID, req.To, req.Caption, req.Filename, req.Mime, req.URL)
	if err != nil {
		s.fail(c, err, "failed to send document message")
		return
	}
	respond(c, http.StatusOK, sendResponse{Success: true, MessageID: res.MessageID, Timestamp: res.Timestamp})
}

func (s *Server) badRequest(c *gin.Context, err error) {
	if isBodyTooLarge(err) {
		respondError(c, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
		return
	}
	respondError(c, http.StatusBadRequest, "validation_error", err.Error())
}

func (s *Server) mediaError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, message.ErrMediaTooLarge):
		respondError(c, http.StatusRequestEntityTooLarge, "media_too_large", "media file is too large")
	case errors.Is(err, message.ErrUnsupportedMediaType):
		respondError(c, http.StatusUnsupportedMediaType, "unsupported_media_type", err.Error())
	default:
		s.badRequest(c, err)
	}
}

func isMultipart(c *gin.Context) bool {
	contentType := c.GetHeader("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.HasPrefix(mediaType, "multipart/")
}

type mediaUpload struct {
	Fields   map[string]string
	Data     []byte
	Filename string
}

const maxFieldSize = 4096

// readMultipartUpload streams the multipart body into memory, never touching
// the disk, and enforces the configured size limit on the file part.
func readMultipartUpload(c *gin.Context, maxBytes int64) (*mediaUpload, error) {
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("invalid multipart body: %w", err)
	}
	upload := &mediaUpload{Fields: make(map[string]string)}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart body: %w", err)
		}
		name := part.FormName()
		if name == "file" {
			data, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
			_ = part.Close()
			if err != nil {
				return nil, fmt.Errorf("read file part: %w", err)
			}
			if int64(len(data)) > maxBytes {
				return nil, message.ErrMediaTooLarge
			}
			upload.Data = data
			upload.Filename = part.FileName()
			continue
		}
		buf, err := io.ReadAll(io.LimitReader(part, maxFieldSize))
		_ = part.Close()
		if err != nil {
			return nil, fmt.Errorf("read form field: %w", err)
		}
		upload.Fields[name] = string(buf)
	}
	if len(upload.Data) == 0 {
		return nil, errors.New("file part is required")
	}
	return upload, nil
}
