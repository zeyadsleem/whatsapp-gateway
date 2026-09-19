package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (c *Client) requireConnected() error {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return ErrInstanceClosed
	}
	if c.wa.Store.ID == nil {
		return ErrNotPaired
	}
	if !c.wa.IsConnected() {
		return ErrNotConnected
	}
	return nil
}

func (c *Client) send(ctx context.Context, to string, msg *waE2E.Message) (*SendResult, error) {
	if err := c.requireConnected(); err != nil {
		return nil, err
	}
	jid := types.NewJID(to, types.DefaultUserServer)
	reqCtx, cancel := context.WithTimeout(ctx, c.sendTimeout)
	defer cancel()

	resp, err := c.wa.SendMessage(reqCtx, jid, msg)
	if err != nil {
		c.sentFailed.Add(1)
		return nil, fmt.Errorf("send whatsapp message: %w", err)
	}
	c.sentOK.Add(1)
	return &SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp}, nil
}

func (c *Client) SendText(ctx context.Context, to, text string) (*SendResult, error) {
	if text == "" {
		return nil, errors.New("text is empty")
	}
	return c.send(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
}

func (c *Client) SendImage(ctx context.Context, to string, data []byte, mime, caption string) (*SendResult, error) {
	if err := c.requireConnected(); err != nil {
		return nil, err
	}
	uploadCtx, cancel := context.WithTimeout(ctx, c.sendTimeout)
	defer cancel()

	up, err := c.wa.Upload(uploadCtx, data, whatsmeow.MediaImage)
	if err != nil {
		c.sentFailed.Add(1)
		return nil, fmt.Errorf("upload image: %w", err)
	}
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption:       proto.String(caption),
		Mimetype:      proto.String(mime),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}}
	sendCtx, cancelSend := context.WithTimeout(ctx, c.sendTimeout)
	defer cancelSend()
	resp, err := c.wa.SendMessage(sendCtx, types.NewJID(to, types.DefaultUserServer), msg)
	if err != nil {
		c.sentFailed.Add(1)
		return nil, fmt.Errorf("send image: %w", err)
	}
	c.sentOK.Add(1)
	return &SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp}, nil
}

func (c *Client) SendDocument(ctx context.Context, to string, data []byte, filename, mime, caption string) (*SendResult, error) {
	if err := c.requireConnected(); err != nil {
		return nil, err
	}
	if filename == "" {
		filename = "document"
	}
	uploadCtx, cancel := context.WithTimeout(ctx, c.sendTimeout)
	defer cancel()

	up, err := c.wa.Upload(uploadCtx, data, whatsmeow.MediaDocument)
	if err != nil {
		c.sentFailed.Add(1)
		return nil, fmt.Errorf("upload document: %w", err)
	}
	msg := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		Caption:       proto.String(caption),
		FileName:      proto.String(filename),
		Mimetype:      proto.String(mime),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
	}}
	sendCtx, cancelSend := context.WithTimeout(ctx, c.sendTimeout)
	defer cancelSend()
	resp, err := c.wa.SendMessage(sendCtx, types.NewJID(to, types.DefaultUserServer), msg)
	if err != nil {
		c.sentFailed.Add(1)
		return nil, fmt.Errorf("send document: %w", err)
	}
	c.sentOK.Add(1)
	return &SendResult{MessageID: resp.ID, Timestamp: resp.Timestamp}, nil
}

func (c *Client) LastActivity() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.qrExpiresAt.IsZero() {
		return time.Time{}
	}
	return c.qrExpiresAt
}
