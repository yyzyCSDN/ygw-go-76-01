package rule

import "sync"

type ChangeHandler func(gateGroup string, current Rule)

type ChangeNotifier struct {
	mu     sync.RWMutex
	nextID int
	subs   map[int]ChangeHandler
}

func NewChangeNotifier() *ChangeNotifier {
	return &ChangeNotifier{
		nextID: 1,
		subs:   make(map[int]ChangeHandler),
	}
}

func (n *ChangeNotifier) Subscribe(handler ChangeHandler) func() {
	n.mu.Lock()
	id := n.nextID
	n.nextID++
	n.subs[id] = handler
	n.mu.Unlock()
	return func() {
		n.mu.Lock()
		delete(n.subs, id)
		n.mu.Unlock()
	}
}

func (n *ChangeNotifier) Broadcast(gateGroup string, current Rule) {
	n.mu.RLock()
	handlers := make([]ChangeHandler, 0, len(n.subs))
	for _, handler := range n.subs {
		handlers = append(handlers, handler)
	}
	n.mu.RUnlock()
	for _, handler := range handlers {
		handler(gateGroup, current)
	}
}
