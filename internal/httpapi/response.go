package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zeyad/whatsapp-gateway/internal/database"
	"github.com/zeyad/whatsapp-gateway/internal/instance"
	"github.com/zeyad/whatsapp-gateway/internal/message"
	"github.com/zeyad/whatsapp-gateway/internal/whatsapp"
)

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

func respond(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

func respondError(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, errorBody{Error: errorDetail{Code: code, Message: msg}})
}

func bindStrict(c *gin.Context, dst any) error {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return err != nil && strings.Contains(err.Error(), "request body too large")
}

func domainErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, whatsapp.ErrNotFound), errors.Is(err, message.ErrInstanceNotFound),
		errors.Is(err, database.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, whatsapp.ErrNotConnected), errors.Is(err, whatsapp.ErrNotPaired):
		return http.StatusConflict, "instance_not_connected"
	case errors.Is(err, whatsapp.ErrAlreadyPaired), errors.Is(err, whatsapp.ErrPairingInProgress):
		return http.StatusConflict, "pairing_conflict"
	case errors.Is(err, whatsapp.ErrMaxInstances):
		return http.StatusConflict, "max_instances_reached"
	case errors.Is(err, whatsapp.ErrInstanceClosed):
		return http.StatusConflict, "instance_closed"
	case errors.Is(err, database.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, instance.ErrValidation):
		return http.StatusBadRequest, "validation_error"
	case errors.Is(err, message.ErrInvalidPhone):
		return http.StatusBadRequest, "invalid_phone"
	case errors.Is(err, message.ErrEmptyText), errors.Is(err, message.ErrTextTooLong):
		return http.StatusBadRequest, "invalid_text"
	case errors.Is(err, message.ErrMediaTooLarge):
		return http.StatusRequestEntityTooLarge, "media_too_large"
	case errors.Is(err, message.ErrUnsupportedMediaType):
		return http.StatusUnsupportedMediaType, "unsupported_media_type"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func (s *Server) fail(c *gin.Context, err error, fallbackMsg string) {
	status, code := domainErrorStatus(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		s.log.Error("request failed", "request_id", requestID(c), "event", "request", "result", "failure", "error", err)
		msg = fallbackMsg
	}
	respondError(c, status, code, msg)
}
