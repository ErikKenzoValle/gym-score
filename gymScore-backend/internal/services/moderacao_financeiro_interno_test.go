package services

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	mysqldrv "github.com/go-sql-driver/mysql"
)

func TestTipoLedgerDoPendente(t *testing.T) {
	casos := map[string]models.TipoTransacao{
		models.PendentePremio:             models.TipoPremioDesafio,
		models.PendentePagamentoInstrutor: models.TipoPagamentoInstrutor,
		models.PendenteReceitaAcademia:    models.TipoReceitaAcademia,
	}
	for p, want := range casos {
		if got, rot, ok := tipoLedgerDoPendente(p); !ok || got != want || rot == "" {
			t.Errorf("%s -> %s (%v)", p, got, ok)
		}
	}
	// a taxa vai para plataforma_receitas, nunca para o ledger de usuário
	if _, _, ok := tipoLedgerDoPendente(models.PendenteTaxaInscricao); ok {
		t.Error("taxa_inscricao não tem tipo de ledger")
	}
}

func TestVencido_RelancamentoContaDaAnulacao(t *testing.T) {
	agora := time.Now()
	antigo := agora.Add(-10 * 24 * time.Hour)
	anulado := agora.Add(-time.Hour)
	modo := models.AnulacaoRelancarPodio
	d := &models.Desafio{Status: models.StatusEmAndamento, DataEvento: &antigo, ResultadoAnuladoEm: &anulado, ResultadoAnulacaoModo: &modo}
	if vencido(d, agora, 48*time.Hour) {
		t.Error("aguardando relançamento: a carência conta da anulação (1h atrás)")
	}
	velho := agora.Add(-72 * time.Hour)
	d.ResultadoAnuladoEm = &velho
	if !vencido(d, agora, 48*time.Hour) {
		t.Error("anulação há 72h com carência de 48h deveria vencer")
	}
	d.ResultadoAnuladoEm, d.ResultadoAnulacaoModo = nil, nil
	if !vencido(d, agora, 48*time.Hour) {
		t.Error("sem anulação vale a data do evento")
	}
	d.Status = models.StatusEncerrado
	if vencido(d, agora, 48*time.Hour) {
		t.Error("encerrado nunca vence")
	}
}

func TestErroSaqueComDebito(t *testing.T) {
	e := ErroSaqueComDebito(12345)
	if e.Status != http.StatusConflict || e.Codigo != CodigoDebitoAberto {
		t.Errorf("%+v", e)
	}
	if m, ok := e.Dados.(map[string]any); !ok || m["debito_aberto_centavos"] != int64(12345) {
		t.Errorf("dados: %v", e.Dados)
	}
}

func TestComRetentativaDeadlock(t *testing.T) {
	n := 0
	err := comRetentativaDeadlock(func() error {
		n++
		if n < 2 {
			return &mysqldrv.MySQLError{Number: 1213}
		}
		return nil
	})
	if err != nil || n != 2 {
		t.Errorf("deadlock deveria ser repetido: n=%d err=%v", n, err)
	}
	n = 0
	outro := errors.New("outro")
	if err := comRetentativaDeadlock(func() error { n++; return outro }); err != outro || n != 1 {
		t.Errorf("erro comum não repete: n=%d", n)
	}
	n = 0
	if err := comRetentativaDeadlock(func() error { n++; return &mysqldrv.MySQLError{Number: 1213} }); !repositories.EhDeadlock(err) || n != tentativasDeadlock {
		t.Errorf("desiste depois de %d tentativas: n=%d", tentativasDeadlock, n)
	}
}

func TestEstadoResultado_RelancamentoSoDono(t *testing.T) {
	em := time.Now()
	modo := models.AnulacaoRelancarPodio
	d := &models.Desafio{ID: 1, Status: models.StatusEmAndamento, IDCriador: 10, ResultadoAnuladoEm: &em, ResultadoAnulacaoModo: &modo}
	resp := &models.DesafioResponse{PodeLancarPodio: true}
	estadoResultado(resp, d, &contextoUsuario{ID: 20})
	if resp.PodeLancarPodio || !resp.ResultadoAnulado || !resp.AguardandoRelancamento {
		t.Errorf("não dono: %+v", resp)
	}
}
