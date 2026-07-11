package transportbody

import "sync"

type Budget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

type Reservation struct {
	budget *Budget
	once   sync.Once
	mu     sync.Mutex
	bytes  int64
}

func NewBudget(limit int64) *Budget {
	if limit <= 0 {
		limit = DefaultLimits().MemoryBudgetBytes
	}
	return &Budget{limit: limit}
}

func (budget *Budget) Reserve(bytes int64) (*Reservation, bool) {
	if bytes < 0 {
		return nil, false
	}
	if bytes == 0 {
		return &Reservation{}, true
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if bytes > budget.limit-budget.used {
		return nil, false
	}
	budget.used += bytes
	return &Reservation{budget: budget, bytes: bytes}, true
}

func (budget *Budget) Used() int64 {
	if budget == nil {
		return 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.used
}

func (budget *Budget) Limit() int64 {
	if budget == nil {
		return 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.limit
}

func (budget *Budget) Available() int64 {
	if budget == nil {
		return 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.limit - budget.used
}

func (reservation *Reservation) ShrinkTo(bytes int64) {
	if reservation == nil || bytes < 0 {
		return
	}
	reservation.mu.Lock()
	defer reservation.mu.Unlock()
	if reservation.budget == nil || bytes >= reservation.bytes {
		return
	}
	reservation.budget.mu.Lock()
	reservation.budget.used -= reservation.bytes - bytes
	reservation.budget.mu.Unlock()
	reservation.bytes = bytes
}

func (reservation *Reservation) Release() {
	if reservation == nil {
		return
	}
	reservation.once.Do(func() {
		reservation.mu.Lock()
		defer reservation.mu.Unlock()
		if reservation.budget == nil || reservation.bytes == 0 {
			return
		}
		reservation.budget.mu.Lock()
		reservation.budget.used -= reservation.bytes
		reservation.budget.mu.Unlock()
		reservation.bytes = 0
	})
}
