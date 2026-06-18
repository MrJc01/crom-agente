package calculator

import (
	"sync"
)

// Counter é um contador thread-safe... ou deveria ser
type Counter struct {
	mu    sync.Mutex
	value int
}

// NewCounter cria um novo counter
func NewCounter() *Counter {
	return &Counter{}
}

// BUG: Race condition — Increment lê e escreve sem lock
func (c *Counter) Increment() {
	// BUG: Falta c.mu.Lock() e defer c.mu.Unlock()
	c.value++
}

// BUG: Deadlock potencial — chama Increment dentro do lock
func (c *Counter) IncrementBy(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < n; i++ {
		c.Increment() // BUG: Increment tenta adquirir o lock novamente = deadlock se tivesse lock
	}
}

// Value retorna o valor atual
func (c *Counter) Value() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// BUG: Goroutine leak — canal nunca é consumido em certos paths
func AsyncIncrement(c *Counter, n int) chan bool {
	done := make(chan bool) // BUG: canal sem buffer, pode travar se ninguém ler
	go func() {
		for i := 0; i < n; i++ {
			c.Increment()
		}
		done <- true
	}()
	return done
}
