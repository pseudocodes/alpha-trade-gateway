// Package trader EventBus 事件总线
// 支持事件订阅/发布、优先级排序、Before 事件拦截链
package trader

import (
	"alpha-trade-gateway/pkg/logger"
	"sort"
	"sync"

	"go.uber.org/zap"
)

// 优先级常量
const (
	PriorityHighest = 0
	PriorityHigh    = 10
	PriorityNormal  = 50
	PriorityLow     = 90
	PriorityLowest  = 100
)

// EventHandler 事件处理函数
type EventHandler func(event *Event)

// EventInterceptor Before 事件拦截器
// 返回 (allow bool, reason string)
type EventInterceptor func(event *Event) (bool, string)

// eventSubscription 事件订阅条目
type eventSubscription struct {
	handler  EventHandler
	priority int
}

// interceptorSubscription 拦截器订阅条目
type interceptorSubscription struct {
	interceptor EventInterceptor
	priority    int
}

// EventBus 事件总线
type EventBus struct {
	mu           sync.RWMutex
	handlers     map[EventType][]eventSubscription
	interceptors map[EventType][]interceptorSubscription
}

// NewEventBus 创建事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		handlers:     make(map[EventType][]eventSubscription),
		interceptors: make(map[EventType][]interceptorSubscription),
	}
}

// Subscribe 订阅事件
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler, priority int) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], eventSubscription{
		handler:  handler,
		priority: priority,
	})

	// 按优先级排序 (数字越小越先执行)
	sort.Slice(eb.handlers[eventType], func(i, j int) bool {
		return eb.handlers[eventType][i].priority < eb.handlers[eventType][j].priority
	})
}

// SubscribeInterceptor 注册拦截器 (仅 Before 事件)
func (eb *EventBus) SubscribeInterceptor(eventType EventType, interceptor EventInterceptor, priority int) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.interceptors[eventType] = append(eb.interceptors[eventType], interceptorSubscription{
		interceptor: interceptor,
		priority:    priority,
	})

	// 按优先级排序
	sort.Slice(eb.interceptors[eventType], func(i, j int) bool {
		return eb.interceptors[eventType][i].priority < eb.interceptors[eventType][j].priority
	})
}

// Publish 发布事件 (同步分发给所有订阅者)
// 非 Before 事件使用此方法
func (eb *EventBus) Publish(event *Event) {
	eb.mu.RLock()
	subs := eb.handlers[event.Type]
	eb.mu.RUnlock()

	for _, sub := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if logger.L != nil {
						logger.Error("event handler panic",
							zap.String("event", event.Type.String()),
							zap.Any("panic", r),
						)
					}
				}
			}()
			sub.handler(event)
		}()
	}
}

// PublishSync 同步发布事件 (用于 Before 事件，需要等待拦截结果)
// 先执行拦截器链，任一拦截器拒绝则短路；
// 拦截器全部通过后，再分发给普通订阅者。
func (eb *EventBus) PublishSync(event *Event) *Event {
	eb.mu.RLock()
	interceptors := eb.interceptors[event.Type]
	subs := eb.handlers[event.Type]
	eb.mu.RUnlock()

	// 执行拦截器链
	for _, ic := range interceptors {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if logger.L != nil {
						logger.Error("event interceptor panic",
							zap.String("event", event.Type.String()),
							zap.Any("panic", r),
						)
					}
					// panic 视为拒绝
					event.Rejected = true
					event.RejectMsg = "interceptor panic"
				}
			}()
			allow, reason := ic.interceptor(event)
			if !allow {
				event.Rejected = true
				event.RejectMsg = reason
			}
		}()

		// 短路: 一旦被拒绝，后续拦截器不再执行
		if event.Rejected {
			return event
		}
	}

	// 拦截器全部通过，分发给普通订阅者
	for _, sub := range subs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if logger.L != nil {
						logger.Error("event handler panic",
							zap.String("event", event.Type.String()),
							zap.Any("panic", r),
						)
					}
				}
			}()
			sub.handler(event)
		}()
	}

	return event
}
