// Package mlclient implementa o client HTTP do serviço de ML e um circuit
// breaker simples para protegê-lo. Princípio do projeto: o ML é
// enriquecimento, nunca ponto único de falha — se o serviço cair, o Go
// degrada graciosamente em vez de propagar erro.
package mlclient

import (
	"sync"
	"time"
)

// state representa os três estados do circuit breaker.
type state int

const (
	stateClosed state = iota
	stateOpen
	stateHalfOpen
)

// CircuitBreaker é um circuit breaker de 3 estados (closed, open, half-open),
// seguro para uso concorrente. Abre após failureThreshold falhas
// consecutivas; permanece aberto por openDuration; depois disso passa a
// half-open e permite uma única chamada de teste — sucesso fecha o
// circuito, falha reabre (reiniciando o temporizador de openDuration).
type CircuitBreaker struct {
	mu sync.Mutex

	failureThreshold int
	openDuration     time.Duration

	state               state
	consecutiveFailures int
	openedAt            time.Time

	// now permite injetar um clock determinístico em testes.
	now func() time.Time
}

// NewCircuitBreaker cria um CircuitBreaker fechado. failureThreshold deve
// ser >= 1; um valor <= 0 é tratado como 1 para evitar um circuito que
// nunca abre.
func NewCircuitBreaker(failureThreshold int, openDuration time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 1
	}

	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
		state:            stateClosed,
		now:              time.Now,
	}
}

// Allow reporta se uma chamada pode ser tentada agora. Retorna false quando
// o circuito está aberto e openDuration ainda não passou. Quando
// openDuration já passou, transiciona o circuito para half-open e permite a
// tentativa (que decidirá, via RecordSuccess/RecordFailure, se o circuito
// fecha ou reabre).
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed, stateHalfOpen:
		return true
	case stateOpen:
		if cb.now().Sub(cb.openedAt) >= cb.openDuration {
			cb.state = stateHalfOpen
			return true
		}
		return false
	default:
		return true
	}
}

// RecordSuccess registra o sucesso de uma chamada previamente autorizada por
// Allow. Em half-open, fecha o circuito. Em closed, zera o contador de
// falhas consecutivas.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = stateClosed
	cb.consecutiveFailures = 0
}

// RecordFailure registra a falha de uma chamada previamente autorizada por
// Allow. Em half-open, reabre o circuito imediatamente (reiniciando o
// temporizador). Em closed, incrementa o contador de falhas consecutivas e
// abre o circuito ao atingir failureThreshold.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateHalfOpen:
		cb.state = stateOpen
		cb.openedAt = cb.now()
		cb.consecutiveFailures = 0
	default:
		cb.consecutiveFailures++
		if cb.consecutiveFailures >= cb.failureThreshold {
			cb.state = stateOpen
			cb.openedAt = cb.now()
			cb.consecutiveFailures = 0
		}
	}
}
