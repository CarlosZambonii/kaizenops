package ingest

import (
	"context"
	"sync"
)

// Task é uma unidade de trabalho enfileirada no pool. Deve observar
// ctx.Done() se fizer IO potencialmente longo, para permitir cancelamento.
type Task func(ctx context.Context) error

// Pool é um worker pool simples: goroutines fixas consumindo de um channel
// buffered.
type Pool struct {
	ctx     context.Context
	tasks   chan Task
	wg      sync.WaitGroup
	onError func(error)
}

// NewPool inicia workers goroutines lendo de um channel com capacidade
// bufferSize. ctx é propagado para cada Task; onError é chamado (se não nil)
// quando uma Task retorna erro.
func NewPool(ctx context.Context, workers, bufferSize int, onError func(error)) *Pool {
	p := &Pool{
		ctx:     ctx,
		tasks:   make(chan Task, bufferSize),
		onError: onError,
	}

	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.runWorker()
	}

	return p
}

func (p *Pool) runWorker() {
	defer p.wg.Done()

	for task := range p.tasks {
		if err := task(p.ctx); err != nil && p.onError != nil {
			p.onError(err)
		}
	}
}

// Submit enfileira uma Task. Bloqueia se o buffer estiver cheio, até que haja
// espaço ou o ctx passado seja cancelado.
func (p *Pool) Submit(ctx context.Context, task Task) error {
	select {
	case p.tasks <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown para de aceitar novas tasks e espera as já enfileiradas
// terminarem. Chamadas a Submit após Shutdown entram em pânico, como o
// comportamento padrão de enviar em um channel fechado — pare de submeter
// antes de chamar Shutdown.
func (p *Pool) Shutdown() {
	close(p.tasks)
	p.wg.Wait()
}
