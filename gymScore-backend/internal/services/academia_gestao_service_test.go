package services

import (
	"strings"
	"testing"

	"gymScore-backend/internal/models"
)

func TestPercentual(t *testing.T) {
	casos := []struct {
		parte, total int64
		want         float64
	}{
		{0, 0, 0}, {5, 0, 0}, {0, 4, 0}, {2, 4, 50}, {1, 3, 33.3}, {2, 3, 66.7}, {4, 4, 100},
	}
	for _, c := range casos {
		if got := percentual(c.parte, c.total); got != c.want {
			t.Errorf("percentual(%d,%d) = %v, esperava %v", c.parte, c.total, got, c.want)
		}
	}
}

func TestStatusValido(t *testing.T) {
	for _, s := range []models.StatusDesafio{"aberto", "em_andamento", "encerrado", "cancelado"} {
		if !statusValido(s) {
			t.Errorf("%q deveria ser válido", s)
		}
	}
	for _, s := range []models.StatusDesafio{"", "em_votacao", "ABERTO", "x"} {
		if statusValido(s) {
			t.Errorf("%q não deveria ser válido", s)
		}
	}
}

func TestMensagemBloqueioDesvinculoLimitaOsDesafiosCitados(t *testing.T) {
	ds := []models.Desafio{{ID: 1, Titulo: "A"}, {ID: 2, Titulo: "B"}, {ID: 3, Titulo: "C"}, {ID: 4, Titulo: "D"}}
	msg := mensagemBloqueioDesvinculo(ds)
	for _, quer := range []string{"#1 A", "#2 B", "#3 C", "..."} {
		if !strings.Contains(msg, quer) {
			t.Errorf("mensagem sem %q: %s", quer, msg)
		}
	}
	if strings.Contains(msg, "#4") {
		t.Errorf("mensagem cita o 4º desafio: %s", msg)
	}
	if curta := mensagemBloqueioDesvinculo(ds[:1]); strings.Contains(curta, "...") {
		t.Errorf("com 1 desafio não deveria ter reticências: %s", curta)
	}
}
