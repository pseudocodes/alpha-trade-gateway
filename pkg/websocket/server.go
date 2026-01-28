// Package websocket WebSocket 服务器
// 对应 C++ open-trade-gateway/connection.h/cpp 和 trade_server.h/cpp
package websocket

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
	"alpha-trade-gateway/pkg/protocol"
)

// TraderHandler 交易处理器接口
type TraderHandler interface {
	// OnConnect 新连接建立
	OnConnect(connID int)
	// OnDisconnect 连接断开
	OnDisconnect(connID int)
	// OnMessage 收到消息
	OnMessage(connID int, msg string)
	// SendMsg 向连接发送消息
	SetMsgSender(sender func(connID int, msg string))
}

// Server WebSocket 服务器
// 对应 C++ trade_server 类
type Server struct {
	ctx        context.Context
	cancel     context.CancelFunc
	host       string
	port       int
	handler    TraderHandler
	httpServer *http.Server

	// 连接管理
	// 对应 C++ connection_manager
	connections sync.Map // map[int]*Connection
	connIDGen   atomic.Int32
}

// NewServer 创建新的 WebSocket 服务器
func NewServer(ctx context.Context, host string, port int, handler TraderHandler) *Server {
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{
		ctx:     ctx,
		cancel:  cancel,
		host:    host,
		port:    port,
		handler: handler,
	}

	// 设置消息发送回调
	handler.SetMsgSender(s.SendMsg)

	return s
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	logger.Info("websocket server starting", zap.String("addr", addr))

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return s.httpServer.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop() {
	logger.Info("websocket server stopping")
	s.cancel()

	// 关闭所有连接
	s.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok {
			conn.Close()
		}
		return true
	})

	// 关闭 HTTP 服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx)
}

// handleWebSocket 处理 WebSocket 连接
// 对应 C++ connection::OnOpenConnection
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 升级到 WebSocket
	// coder/websocket 使用 Accept 函数而不是 Upgrader
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 允许所有来源，对应之前的 CheckOrigin
	})
	if err != nil {
		logger.Error("websocket upgrade failed", zap.Error(err))
		return
	}

	// 生成连接 ID
	connID := int(s.connIDGen.Add(1))

	// 获取客户端 IP
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	// 创建连接对象
	conn := NewConnection(s.ctx, connID, wsConn, clientIP, s)
	s.connections.Store(connID, conn)

	logger.Info("websocket connection established",
		zap.Int("conn_id", connID),
		zap.String("client_ip", clientIP),
	)

	// 通知 handler
	s.handler.OnConnect(connID)

	// 开始读取消息
	go conn.ReadLoop()
}

// SendMsg 向指定连接发送消息
// 对应 C++ connection::SendTextMsg
func (s *Server) SendMsg(connID int, msg string) {
	if val, ok := s.connections.Load(connID); ok {
		if conn, ok := val.(*Connection); ok {
			conn.Send(msg)
		}
	}
}

// BroadcastMsg 广播消息到所有连接
func (s *Server) BroadcastMsg(msg string) {
	s.connections.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*Connection); ok {
			conn.Send(msg)
		}
		return true
	})
}

// removeConnection 移除连接
func (s *Server) removeConnection(connID int) {
	s.connections.Delete(connID)
	s.handler.OnDisconnect(connID)
}

// Connection WebSocket 连接
// 对应 C++ connection 类
type Connection struct {
	ctx      context.Context
	cancel   context.CancelFunc
	id       int
	conn     *websocket.Conn
	clientIP string
	server   *Server

	// 写入队列
	sendChan chan string
	closed   atomic.Bool
}

// NewConnection 创建新连接
func NewConnection(ctx context.Context, id int, conn *websocket.Conn, clientIP string, server *Server) *Connection {
	ctx, cancel := context.WithCancel(ctx)
	c := &Connection{
		ctx:      ctx,
		cancel:   cancel,
		id:       id,
		conn:     conn,
		clientIP: clientIP,
		server:   server,
		sendChan: make(chan string, 256),
	}

	// 启动写入 goroutine
	go c.writeLoop()

	return c
}

// ID 返回连接 ID
func (c *Connection) ID() int {
	return c.id
}

// ReadLoop 读取消息循环
// 对应 C++ connection::DoRead
func (c *Connection) ReadLoop() {
	defer func() {
		c.Close()
		c.server.removeConnection(c.id)
		logger.Info("websocket connection closed", zap.Int("conn_id", c.id))
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// 读取消息
		// coder/websocket 使用 Read 方法，返回 MessageType 和数据
		msgType, message, err := c.conn.Read(c.ctx)
		if err != nil {
			// coder/websocket 使用 CloseStatus 来检查关闭状态
			status := websocket.CloseStatus(err)
			if status != websocket.StatusNormalClosure && status != websocket.StatusGoingAway {
				logger.Error("websocket read error",
					zap.Int("conn_id", c.id),
					zap.Error(err),
				)
			}
			return
		}

		// 只处理文本消息
		if msgType != websocket.MessageText {
			continue
		}

		// 处理消息
		// 对应 C++ connection::OnMessage
		msg := string(message)
		logger.Debug("received message",
			zap.Int("conn_id", c.id),
			zap.String("msg", truncateMsg(msg, 200)),
		)

		c.server.handler.OnMessage(c.id, msg)
	}
}

// writeLoop 写入消息循环
// 对应 C++ connection::DoWrite
func (c *Connection) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.sendChan:
			if c.closed.Load() {
				return
			}
			// coder/websocket 使用 Write 方法，需要传入 context 和 MessageType
			if err := c.conn.Write(c.ctx, websocket.MessageText, []byte(msg)); err != nil {
				logger.Error("websocket write error",
					zap.Int("conn_id", c.id),
					zap.Error(err),
				)
				return
			}
		}
	}
}

// Send 发送消息
func (c *Connection) Send(msg string) {
	if c.closed.Load() {
		return
	}
	select {
	case c.sendChan <- msg:
	default:
		logger.Warn("send channel full, dropping message",
			zap.Int("conn_id", c.id),
		)
	}
}

// Close 关闭连接
func (c *Connection) Close() {
	if c.closed.Swap(true) {
		return
	}
	c.cancel()
	// coder/websocket 使用 Close 方法，需要传入 status code 和 reason
	c.conn.Close(websocket.StatusNormalClosure, "connection closed")
}

// 辅助函数：截断消息用于日志
func truncateMsg(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen] + "..."
}

// ReqLogin 结构定义 (用于解析登录请求)
type ReqLoginMsg struct {
	Aid      string               `json:"aid"`
	Bid      string               `json:"bid"`
	UserName string               `json:"user_name"`
	Password string               `json:"password"`
	Broker   protocol.BrokerLogin `json:"broker,omitempty"`
}
