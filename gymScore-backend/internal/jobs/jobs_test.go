package jobs

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgendador_RodaEPara(t *testing.T) {
	var n atomic.Int32
	a := Iniciar(10*time.Millisecond,
		Tarefa{Nome: "conta", Executar: func() error { n.Add(1); return nil }},
		Tarefa{Nome: "falha", Executar: func() error { return errors.New("x") }},
		Tarefa{Nome: "panico", Executar: func() error { panic("boom") }},
	)
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	a.Parar()
	depois := n.Load()
	if depois < 3 {
		t.Fatalf("esperava >= 3 execuções, houve %d", depois)
	}
	time.Sleep(40 * time.Millisecond)
	if n.Load() != depois {
		t.Error("o agendador continuou rodando depois de Parar")
	}
	a.Parar() // idempotente
}

func TestAgendador_IntervaloZeroNaoInicia(t *testing.T) {
	var n atomic.Int32
	a := Iniciar(0, Tarefa{Nome: "x", Executar: func() error { n.Add(1); return nil }})
	time.Sleep(20 * time.Millisecond)
	a.Parar()
	if n.Load() != 0 {
		t.Error("intervalo 0 não deveria executar")
	}
}
