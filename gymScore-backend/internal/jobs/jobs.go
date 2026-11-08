// Package jobs roda tarefas periódicas em background (expiração de desafios, reconciliação PIX).
// Cada tarefa precisa ser idempotente e segura com várias instâncias rodando ao mesmo tempo: a
// coordenação fica no banco (trava de linha + UPDATE condicional), não aqui.
package jobs

import (
	"context"
	"log"
	"sync"
	"time"
)

// Tarefa é um passo executado a cada tick.
type Tarefa struct {
	Nome     string
	Executar func() error
}

// Agendador executa as tarefas em sequência a cada intervalo até Parar.
type Agendador struct {
	cancelar context.CancelFunc
	wg       sync.WaitGroup
}

// Iniciar dispara as tarefas a cada `intervalo` (a primeira execução acontece após um intervalo).
// intervalo <= 0 não inicia nada. Pânico numa tarefa é registrado e não derruba o processo.
func Iniciar(intervalo time.Duration, tarefas ...Tarefa) *Agendador {
	a := &Agendador{}
	if intervalo <= 0 || len(tarefas) == 0 {
		return a
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelar = cancel
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(intervalo)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, t := range tarefas {
					if ctx.Err() != nil {
						return
					}
					executar(t)
				}
			}
		}
	}()
	log.Printf("[JOBS] %d tarefa(s) a cada %s", len(tarefas), intervalo)
	return a
}

// ExecutarAgora roda todas as tarefas uma vez, de forma síncrona (testes e uso manual).
func ExecutarAgora(tarefas ...Tarefa) {
	for _, t := range tarefas {
		executar(t)
	}
}

func executar(t Tarefa) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[JOBS] pânico em %s: %v", t.Nome, r)
		}
	}()
	if err := t.Executar(); err != nil {
		log.Printf("[JOBS] %s: %v", t.Nome, err)
	}
}

// Parar interrompe o agendador e espera a rodada em andamento terminar. Idempotente.
func (a *Agendador) Parar() {
	if a == nil || a.cancelar == nil {
		return
	}
	a.cancelar()
	a.wg.Wait()
}
