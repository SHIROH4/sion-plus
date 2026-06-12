// Package qq implements the QQ Bot plugin — WebSocket connection to
// QQ Bot API for message relay and EventBus integration.
package qq

import (
	"context"
	"log"
	"sync"

	"github.com/shirohania/sion/plugin/sdk"
)

// Plugin implements sdk.Plugin for the QQ Bot module.
type Plugin struct {
	sdk.BasePlugin
	pctx    *sdk.PluginContext
	mu      sync.Mutex
	conn    QQConnection
	stopCh  chan struct{}
	msgCh   chan QQMessage
}

// QQConnection abstracts the QQ Bot WebSocket connection.
type QQConnection interface {
	Connect(ctx context.Context) error
	Close() error
	SendMessage(ctx context.Context, targetID, content string) error
	OnMessage(handler func(msg QQMessage))
}

// QQMessage is a received QQ message.
type QQMessage struct {
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	GroupID    string `json:"group_id,omitempty"`
	Content    string `json:"content"`
	Timestamp  int64  `json:"timestamp"`
}

func New() *Plugin {
	return &Plugin{
		BasePlugin: sdk.NewBasePlugin(sdk.PluginInfo{
			Name:        "qq",
			Version:     "1.0.0",
			Description: "QQ Bot message relay via WebSocket",
			Author:      "Sion",
		}),
		stopCh: make(chan struct{}),
		msgCh:  make(chan QQMessage, 100),
	}
}

func (p *Plugin) SetConnection(conn QQConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conn = conn
}

func (p *Plugin) Init(ctx context.Context, pctx *sdk.PluginContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pctx = pctx
	return nil
}

func (p *Plugin) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil {
		return nil
	}
	if err := p.conn.Connect(ctx); err != nil {
		return err
	}

	p.conn.OnMessage(func(msg QQMessage) {
		select {
		case p.msgCh <- msg:
		default:
			// Drop if buffer full
		}
	})

	go p.messageLoop(ctx)
	log.Println("[qq] plugin started")
	return nil
}

func (p *Plugin) messageLoop(ctx context.Context) {
	for {
		select {
		case <-p.stopCh:
			return
		case <-ctx.Done():
			return
		case msg := <-p.msgCh:
			// Relay QQ messages to the proactive pipeline via EventBus
			if p.pctx.EventBus != nil {
				p.pctx.EventBus.Publish("plugin:qq:message", msg)
			}
			// Submit as a proactive intent for the AI to potentially respond
			if p.pctx.IntentSubmitter != nil {
				_ = p.pctx.IntentSubmitter.Submit(sdk.ProactiveIntent{
					Source:   "plugin:qq",
					Action:   "speak_casual",
					Message:  "QQ message from " + msg.SenderName + ": " + msg.Content,
					Priority: 7,
				})
			}
		}
	}
}

func (p *Plugin) Stop(ctx context.Context) error {
	close(p.stopCh)
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

var _ sdk.Plugin = (*Plugin)(nil)
