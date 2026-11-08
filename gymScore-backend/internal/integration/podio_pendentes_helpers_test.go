//go:build integration

package integration

import (
	"sync"
	"testing"

	"gymScore-backend/internal/app"
	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/models"
)

// Pódio com janela de contestação: o lançamento NÃO credita mais o ledger; os créditos viram
// pagamentos_pendentes e o job liberar-pagamentos os credita. Na suíte a janela é 0 (cfgBase), então
// os helpers de ação (acao/acaoS) chamam aposPodio depois de um pódio 200: conferem que o dinheiro
// inteiro do desafio está nos pendentes (e nada a mais no ledger), rodam o job e conferem que tudo que
// não estava retido foi liberado. Os testes antigos continuam verificando os saldos finais de antes.

var (
	liberarOnce   sync.Once
	tarefaLiberar jobs.Tarefa
)

// liberarPagamentos roda uma vez o job liberar-pagamentos (todos os desafios com janela vencida).
func liberarPagamentos(t *testing.T) {
	t.Helper()
	liberarOnce.Do(func() {
		_, tarefas := app.ConstruirComJobs(cfgBase, db, app.Dependencias{Asaas: asaas, Mailer: mailer})
		for _, tf := range tarefas {
			if tf.Nome == "liberar-pagamentos" {
				tarefaLiberar = tf
			}
		}
	})
	if tarefaLiberar.Executar == nil {
		t.Fatal("tarefa liberar-pagamentos não registrada")
	}
	if err := tarefaLiberar.Executar(); err != nil {
		t.Errorf("liberar-pagamentos: %v", err)
	}
}

// somaPendentes soma os pendentes do desafio por status.
func somaPendentes(t *testing.T, id uint) map[string]int64 {
	t.Helper()
	var linhas []struct {
		Status string
		Total  int64
	}
	db.Model(&models.PagamentoPendente{}).Select("status, SUM(valor_centavos) AS total").
		Where("id_desafio = ?", id).Group("status").Scan(&linhas)
	out := map[string]int64{}
	for _, l := range linhas {
		out[l.Status] = l.Total
	}
	return out
}

// conferirPendentesDoPodio: Σ pendentes não cancelados = entradas líquidas + reserva do desafio, e o
// que já está no ledger/receita do pódio é exatamente o que foi liberado.
func conferirPendentesDoPodio(t *testing.T, id uint) {
	t.Helper()
	var d models.Desafio
	if err := db.First(&d, id).Error; err != nil {
		t.Fatal(err)
	}
	var entradas, creditos, taxa int64
	db.Model(&models.Transacao{}).Select("COALESCE(-SUM(valor_centavos),0)").
		Where("id_desafio = ? AND tipo IN ('entrada_desafio','estorno_desafio') AND status IN ('received','concluido')", id).Scan(&entradas)
	db.Model(&models.Transacao{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND tipo IN ('premio_desafio','pagamento_instrutor','receita_academia') AND status IN ('received','concluido')", id).Scan(&creditos)
	db.Model(&models.PlataformaReceita{}).Select("COALESCE(SUM(valor_centavos),0)").
		Where("id_desafio = ? AND origem = 'taxa_inscricao'", id).Scan(&taxa)
	s := somaPendentes(t, id)
	vivos := s[models.PendenteALiberar] + s[models.PendenteRetido] + s[models.PendenteLiberado]
	if vivos != entradas+d.ReservaAcademiaCentavos {
		t.Errorf("desafio %d: Σ pendentes %d != entradas %d + reserva %d", id, vivos, entradas, d.ReservaAcademiaCentavos)
	}
	if creditos+taxa != s[models.PendenteLiberado] {
		t.Errorf("desafio %d: ledger do pódio %d + taxa %d != liberados %d", id, creditos, taxa, s[models.PendenteLiberado])
	}
	if d.PodioLancadoEm == nil || d.PodioLancadoPor == nil {
		t.Errorf("desafio %d: podio_lancado_em/por não gravados", id)
	}
}

// aposPodio é chamado pelos helpers de ação depois de um POST /podio 200.
func aposPodio(t *testing.T, id uint) {
	t.Helper()
	conferirPendentesDoPodio(t, id)
	liberarPagamentos(t)
	conferirPendentesDoPodio(t, id)
	if s := somaPendentes(t, id); s[models.PendenteALiberar] != 0 {
		t.Errorf("desafio %d: %d centavos ainda a_liberar depois do job (janela 0)", id, s[models.PendenteALiberar])
	}
}
