package rule

import (
	"sort"
	"sync"
)

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

// SubIDs returns the current subscriber IDs, in allocation order. It exists
// for tests that need to assert subscription lifecycle (subscribe/unsubscribe).
func (n *ChangeNotifier) SubIDs() []int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	ids := make([]int, 0, len(n.subs))
	for id := range n.subs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
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
