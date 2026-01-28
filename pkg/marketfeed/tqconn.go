package marketfeed

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

type TqConnection struct {
	targetURL string
	conn      *websocket.Conn
	txC       chan string
	rxC       chan string
	headers   http.Header

	reconnC chan struct{}
	quitC   chan struct{}
	closed  atomic.Bool

	OnReconnect func()
}

func NewTqConnection(wsurl string) *TqConnection {
	return &TqConnection{
		targetURL: wsurl,
		txC:       make(chan string, 1024),
		rxC:       make(chan string, 1024),
		reconnC:   make(chan struct{}),
		quitC:     make(chan struct{}),
	}
}

func (c *TqConnection) Connect(ctx context.Context, headers http.Header) error {
	c.headers = headers
	opt := websocket.DialOptions{
		HTTPHeader:      headers,
		CompressionMode: websocket.CompressionContextTakeover,
	}
	conn, _, err := websocket.Dial(ctx, c.targetURL, &opt)
	if err != nil {
		logger.Error("connect website url error",
			zap.String("url", c.targetURL),
			zap.Error(err),
		)
		return err
	}
	conn.SetReadLimit(-1)
	c.conn = conn
	go c.doRead()
	go c.doWrite()
	return nil
}

func (c *TqConnection) doRead() {
	for {
		select {
		case <-c.quitC:
			logger.Info("websocket read loop quit")
			return
		default:
			_, body, err := c.conn.Read(context.Background())
			if err != nil {
				logger.Error("websocket read error", zap.Error(err))
				c.conn.CloseNow()
				c.reconnect()
				return
			}
			c.rxC <- string(body)
		}
	}
}

func (c *TqConnection) doWrite() {
	for {
		select {
		case req := <-c.txC:
			if err := c.conn.Write(context.Background(), websocket.MessageText, []byte(req)); err != nil {
				logger.Error("websocket write error",
					zap.String("pack", req),
					zap.Error(err),
				)
				c.conn.CloseNow()
				return
			}
		case <-c.reconnC:
			logger.Info("reconnect: do send loop exit")
			return
		case <-c.quitC:
			logger.Info("do send loop quit")
			return
		}
	}
}

func (c *TqConnection) reconnect() {
	if c.closed.Load() {
		return
	}
	c.reconnC <- struct{}{}
	for i := 0; ; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err := c.Connect(ctx, c.headers)
		if err == nil {
			cancel()
			break
		}
		dur := 2 * time.Second
		time.Sleep(dur)
		cancel()
	}

	if c.OnReconnect != nil {
		c.OnReconnect()
	}
}

func (c *TqConnection) Tx() chan<- string {
	return c.txC
}

func (c *TqConnection) Rx() <-chan string {
	return c.rxC
}

func (c *TqConnection) Close() {
	close(c.quitC)

	if c.conn != nil {
		c.conn.CloseNow()
	}
	c.closed.Store(true)
}
