package whatsapp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/zeyad/whatsapp-gateway/internal/metrics"
)

func (c *Client) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Connected:
		c.onConnected()
	case *events.Disconnected:
		c.onDisconnected()
	case *events.LoggedOut:
		c.onLoggedOut(v)
	case *events.PairSuccess:
		c.onPairSuccess(v)
	case *events.StreamError:
		c.onStreamError(v)
	case *events.StreamReplaced:
		c.onStreamReplaced()
	case *events.TemporaryBan:
		c.onTemporaryBan(v)
	case *events.ConnectFailure:
		c.onConnectFailure(v)
	case *events.ClientOutdated:
		c.onClientOutdated()
	case *events.KeepAliveTimeout:
		c.onKeepAliveTimeout()
	case *events.KeepAliveRestored:
	case *events.Message:
		c.onMessage(v)
	case *events.HistorySync:
		// Privacy: history sync payloads are intentionally discarded. Nothing is persisted.
	case *events.Contact, *events.PushName, *events.BusinessName, *events.ChatPresence:
		// App-state metadata stays inside the whatsmeow protocol schema.
	default:
	}
}

func (c *Client) onConnected() {
	c.mu.Lock()
	c.manualDisconnect = false
	c.loggedOut = false
	if id := c.wa.Store.ID; id != nil {
		c.waJID = id.String()
		c.phone = id.User
	}
	waJID, phone := c.waJID, c.phone
	c.clearQRLocked()
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), persistTimeout)
	defer cancel()
	// PairSuccess already fills c.phone, so comparing it with the in-memory
	// value would skip the first durable write of the paired identity.
	if phone != "" {
		if err := c.repo.SetIdentity(ctx, c.id, waJID, phone); err != nil {
			c.log.Error("failed to persist instance identity", "event", "persist_identity", "result", "failure", "error", err)
		}
	}
	if err := c.repo.MarkConnected(ctx, c.id, time.Now()); err != nil {
		c.log.Error("failed to mark instance connected", "event", "persist_connected", "result", "failure", "error", err)
	}
	c.setStatus(StatusConnected, nil)
}

func (c *Client) onPairSuccess(evt *events.PairSuccess) {
	c.mu.Lock()
	c.waJID = evt.ID.String()
	c.phone = evt.ID.User
	c.clearQRLocked()
	c.mu.Unlock()
	c.setStatus(StatusConnecting, nil)
}

func (c *Client) onDisconnected() {
	c.mu.RLock()
	manual := c.manualDisconnect
	closed := c.closed
	loggedOut := c.loggedOut
	c.mu.RUnlock()

	if closed || manual || loggedOut {
		return
	}
	c.superviseReconnect()
}

func (c *Client) onLoggedOut(evt *events.LoggedOut) {
	c.mu.Lock()
	c.loggedOut = true
	c.phone = ""
	c.waJID = ""
	c.clearQRLocked()
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), persistTimeout)
	defer cancel()
	if err := c.repo.ClearIdentity(ctx, c.id); err != nil {
		c.log.Error("failed to clear identity after logout", "event", "logout_cleanup", "result", "failure", "error", err)
	}
	c.log.Warn("whatsapp logged out the linked device", "event", "logged_out", "on_connect", evt.OnConnect)
	c.setStatus(StatusLoggedOut, nil)
}

func (c *Client) onStreamError(evt *events.StreamError) {
	c.log.Warn("whatsapp stream error", "event", "stream_error", "code", evt.Code)
	c.setStatus(StatusReconnecting, nil)
	c.superviseReconnect()
}

func (c *Client) onStreamReplaced() {
	c.log.Warn("whatsapp stream replaced by another session; stopping reconnects", "event", "stream_replaced")
	c.mu.Lock()
	c.manualDisconnect = true
	c.mu.Unlock()
	c.setStatus(StatusError, context.Canceled)
}

func (c *Client) onTemporaryBan(evt *events.TemporaryBan) {
	c.log.Warn("whatsapp temporary ban", "event", "temporary_ban", "code", evt.Code.String(), "expires_in", evt.Expire.String())
	c.setStatus(StatusError, context.DeadlineExceeded)
}

func (c *Client) onConnectFailure(evt *events.ConnectFailure) {
	if evt.Reason.IsLoggedOut() {
		c.mu.Lock()
		c.loggedOut = true
		c.phone = ""
		c.waJID = ""
		c.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), persistTimeout)
		defer cancel()
		if err := c.repo.ClearIdentity(ctx, c.id); err != nil {
			c.log.Error("failed to clear identity", "event", "connect_failure_cleanup", "result", "failure", "error", err)
		}
		c.setStatus(StatusLoggedOut, nil)
		return
	}
	c.log.Warn("whatsapp connection failure, will retry", "event", "connect_failure", "reason", evt.Reason.String())
	c.setStatus(StatusReconnecting, nil)
	c.superviseReconnect()
}

func (c *Client) onClientOutdated() {
	c.log.Error("whatsapp client is outdated; update the whatsmeow dependency", "event", "client_outdated")
	c.mu.Lock()
	c.manualDisconnect = true
	c.mu.Unlock()
	c.setStatus(StatusError, context.Canceled)
}

func (c *Client) onKeepAliveTimeout() {
	c.log.Warn("whatsapp keepalive timeout", "event", "keepalive_timeout")
	c.superviseReconnect()
}

func (c *Client) onMessage(evt *events.Message) {
	info := evt.Info
	if info.IsFromMe || info.IsGroup {
		return
	}
	if info.Chat.Server != types.DefaultUserServer && info.Chat.Server != types.HiddenUserServer {
		return
	}
	if info.Category == "peer" {
		return
	}
	msgType, text := extractContent(evt.Message)
	if msgType == "" || msgType == "protocol" || msgType == "other" {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), 2*time.Second)
	from := c.resolveSender(ctx, info)
	cancel()
	if from == "" {
		return
	}

	c.received.Add(1)
	metrics.ReceivedTotal.Inc()
	if c.notifier == nil {
		return
	}
	c.notifier.MessageEvent(MessageEvent{
		InstanceID: c.id,
		ShopID:     c.shopID,
		From:       from,
		MessageID:  info.ID,
		Type:       msgType,
		Text:       text,
		Timestamp:  info.Timestamp,
		EventID:    uuid.NewString(),
	})
}

func (c *Client) resolveSender(ctx context.Context, info types.MessageInfo) string {
	sender := info.Sender
	if sender.Server == types.DefaultUserServer {
		return sender.User
	}
	if alt := info.SenderAlt; !alt.IsEmpty() && alt.Server == types.DefaultUserServer {
		return alt.User
	}
	if sender.Server == types.HiddenUserServer {
		if pn, err := c.wa.Store.LIDs.GetPNForLID(ctx, sender); err == nil && pn.Server == types.DefaultUserServer {
			return pn.User
		}
		return sender.User
	}
	return ""
}

func extractContent(msg *waE2E.Message) (string, string) {
	if msg == nil {
		return "", ""
	}
	switch {
	case msg.GetConversation() != "":
		return "text", msg.GetConversation()
	case msg.GetExtendedTextMessage().GetText() != "":
		return "text", msg.GetExtendedTextMessage().GetText()
	case msg.GetImageMessage() != nil:
		return "image", msg.GetImageMessage().GetCaption()
	case msg.GetVideoMessage() != nil:
		return "video", msg.GetVideoMessage().GetCaption()
	case msg.GetAudioMessage() != nil:
		return "audio", ""
	case msg.GetDocumentMessage() != nil:
		return "document", msg.GetDocumentMessage().GetCaption()
	case msg.GetStickerMessage() != nil:
		return "sticker", ""
	case msg.GetLocationMessage() != nil:
		return "location", ""
	case msg.GetContactMessage() != nil:
		return "contact", ""
	case msg.GetReactionMessage() != nil:
		return "reaction", msg.GetReactionMessage().GetText()
	case msg.GetButtonsResponseMessage() != nil:
		return "buttons_response", msg.GetButtonsResponseMessage().GetSelectedDisplayText()
	case msg.GetListResponseMessage() != nil:
		return "list_response", msg.GetListResponseMessage().GetTitle()
	case msg.GetTemplateButtonReplyMessage() != nil:
		return "template_button_reply", msg.GetTemplateButtonReplyMessage().GetSelectedDisplayText()
	case msg.GetProtocolMessage() != nil:
		return "protocol", ""
	default:
		return "", ""
	}
}
