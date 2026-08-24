package mlclient

import (
	"testing"
	"time"
)

// newTestClock retorna um *time.Time mutável e a função now correspondente,
// para simular passagem de tempo sem sleep real.
func newTestClock(start time.Time) (*time.Time, func() time.Time) {
	t := start
	return &t, func() time.Time { return t }
}

func TestCircuitBreaker_ClosedAllowsAndStaysClosedBelowThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	for i := 0; i < 2; i++ {
		if !cb.Allow() {
			t.Fatalf("iteração %d: esperava Allow()==true com circuito fechado", i)
		}
		cb.RecordFailure()
	}

	if !cb.Allow() {
		t.Fatal("esperava Allow()==true: apenas 2 falhas, threshold é 3")
	}
}

func TestCircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	for i := 0; i < 3; i++ {
		if !cb.Allow() {
			t.Fatalf("iteração %d: esperava Allow()==true antes de atingir threshold", i)
		}
		cb.RecordFailure()
	}

	if cb.Allow() {
		t.Fatal("esperava Allow()==false após 3 falhas consecutivas (threshold=3)")
	}
}

func TestCircuitBreaker_SuccessResetsConsecutiveFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordSuccess() // zera o contador antes de atingir o threshold

	cb.Allow()
	cb.RecordFailure()
	cb.Allow()
	cb.RecordFailure()

	if !cb.Allow() {
		t.Fatal("esperava Allow()==true: contador foi resetado pelo sucesso, só há 2 falhas desde então")
	}
}

func TestCircuitBreaker_HalfOpenAfterOpenDurationElapses(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, now := newTestClock(start)

	cb := NewCircuitBreaker(1, 10*time.Second)
	cb.now = now

	cb.Allow()
	cb.RecordFailure() // abre o circuito

	if cb.Allow() {
		t.Fatal("esperava Allow()==false imediatamente após abrir")
	}

	*clock = start.Add(5 * time.Second)
	if cb.Allow() {
		t.Fatal("esperava Allow()==false: openDuration (10s) ainda não passou")
	}

	*clock = start.Add(10 * time.Second)
	if !cb.Allow() {
		t.Fatal("esperava Allow()==true: openDuration passou, deveria ir para half-open")
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, now := newTestClock(start)

	cb := NewCircuitBreaker(1, 10*time.Second)
	cb.now = now

	cb.Allow()
	cb.RecordFailure() // abre

	*clock = start.Add(10 * time.Second)
	if !cb.Allow() {
		t.Fatal("esperava Allow()==true em half-open")
	}
	cb.RecordSuccess() // fecha o circuito

	// Circuito fechado deve tolerar novamente até failureThreshold falhas.
	*clock = start.Add(10 * time.Second) // sem avanço de tempo, closed não depende do clock
	if !cb.Allow() {
		t.Fatal("esperava Allow()==true: circuito deveria estar fechado após sucesso em half-open")
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock, now := newTestClock(start)

	cb := NewCircuitBreaker(1, 10*time.Second)
	cb.now = now

	cb.Allow()
	cb.RecordFailure() // abre

	*clock = start.Add(10 * time.Second)
	if !cb.Allow() {
		t.Fatal("esperava Allow()==true em half-open")
	}
	cb.RecordFailure() // falha em half-open reabre

	if cb.Allow() {
		t.Fatal("esperava Allow()==false: falha em half-open deve reabrir o circuito")
	}

	// Ainda dentro da nova janela de openDuration, permanece aberto.
	*clock = start.Add(15 * time.Second)
	if cb.Allow() {
		t.Fatal("esperava Allow()==false: openDuration reiniciado pela falha em half-open")
	}

	// Após a nova janela completa (reaberto em t=10s, +10s de openDuration = t=20s), volta a half-open.
	*clock = start.Add(20 * time.Second)
	if !cb.Allow() {
		t.Fatal("esperava Allow()==true: nova janela de openDuration expirou")
	}
}

func TestCircuitBreaker_ConcurrentAccessIsSafe(t *testing.T) {
	cb := NewCircuitBreaker(5, 10*time.Millisecond)

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			if cb.Allow() {
				if i%2 == 0 {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}
