package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Consultas e escritas auxiliares da parte financeira da moderação (pódio com janela de contestação,
// liberação, anulação de resultado e painel). As transições de dinheiro continuam nos repositórios de
// carteira/pendentes/débitos; aqui só o que não pertence a um agregado.

// sqlVencidoAndamento é a condição de vencimento de um desafio `em_andamento`: normalmente data_evento
// + carência; com o resultado anulado aguardando relançamento (Q3) a carência conta da anulação.
const sqlVencidoAndamento = `((resultado_anulacao_modo IS NULL AND data_evento IS NOT NULL AND data_evento < ?)
	OR (resultado_anulacao_modo = 'relancar_podio' AND resultado_anulado_em < ?))`

// RemoverPodio desfaz o pódio vigente (modo relancar_podio): apaga as posições e zera prêmio/vencedor
// dos participantes. O que foi lançado continua registrado no evento podio_lancado da linha do tempo.
func (r *desafioModeracaoRepository) RemoverPodio(tx *gorm.DB, idDesafio uint) error {
	db := dbOuTx(r.db, tx)
	if err := db.Exec("DELETE FROM podio_desafios WHERE id_desafio = ?", idDesafio).Error; err != nil {
		return err
	}
	return db.Exec("UPDATE desafio_participantes SET premio_centavos = 0, vencedor = 0 WHERE id_desafio = ?", idDesafio).Error
}

// UltimoEventoDoDesafio devolve o evento mais recente do desafio com a ação pedida (nil se não houver).
func UltimoEventoDoDesafio(tx *gorm.DB, idDesafio uint, acao string) (*models.ModeracaoEvento, error) {
	var e models.ModeracaoEvento
	err := tx.Where("id_desafio = ? AND entidade = ? AND acao = ?", idDesafio, models.EntidadeDesafio, acao).
		Order("id DESC").First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &e, err
}

// PendenciasModeracao são as filas da moderação no dashboard admin.
type PendenciasModeracao struct {
	DenunciasAdmin       int64 // jurisdição admin, aberta/em_analise
	DenunciasEscalonadas int64 // dessas, as que subiram da academia (encaminhada_em preenchido)
	DenunciasAcademia    int64 // ainda com a academia julgadora (só acompanhamento)
	RecursosPendentes    int64
	PagamentosRetidos    int64 // pendentes retidos por denúncia
	DebitosAbertos       int64 // débitos não quitados (quantidade)
}

// ContarPendenciasModeracao conta as filas da moderação (uma consulta).
func ContarPendenciasModeracao(db *gorm.DB) (PendenciasModeracao, error) {
	var p PendenciasModeracao
	err := db.Raw(`SELECT
		(SELECT COUNT(*) FROM denuncias WHERE jurisdicao = 'admin' AND status IN ('aberta','em_analise')) AS denuncias_admin,
		(SELECT COUNT(*) FROM denuncias WHERE jurisdicao = 'admin' AND status IN ('aberta','em_analise')
		   AND encaminhada_em IS NOT NULL) AS denuncias_escalonadas,
		(SELECT COUNT(*) FROM denuncias WHERE jurisdicao = 'academia' AND status IN ('aberta','em_analise')) AS denuncias_academia,
		(SELECT COUNT(*) FROM recursos WHERE status = 'pendente') AS recursos_pendentes,
		(SELECT COUNT(*) FROM pagamentos_pendentes WHERE status = 'retido') AS pagamentos_retidos,
		(SELECT COUNT(*) FROM usuario_debitos WHERE quitado_em IS NULL) AS debitos_abertos`).Scan(&p).Error
	return p, err
}

// DebitoAbertoDoUsuario soma o saldo restante dos débitos abertos (sem trava; telas e /carteira).
func DebitoAbertoDoUsuario(db *gorm.DB, idUsuario uint) (int64, error) {
	var total int64
	err := db.Model(&models.UsuarioDebito{}).Select("COALESCE(SUM(saldo_restante_centavos), 0)").
		Where("id_usuario = ? AND quitado_em IS NULL", idUsuario).Scan(&total).Error
	return total, err
}

// LiberacaoDoDesafio resume os pendentes de um desafio para a tela do desafio.
type LiberacaoDoDesafio struct {
	ALiberar  int64      // quantos a_liberar
	Retidos   int64      // quantos retidos
	LiberarEm *time.Time // menor liberar_em dos a_liberar/retidos
}

// ResumoLiberacao devolve quantos pendentes do desafio estão a liberar/retidos e quando liberam.
func ResumoLiberacao(db *gorm.DB, idDesafio uint) (LiberacaoDoDesafio, error) {
	var l LiberacaoDoDesafio
	err := db.Raw(`SELECT
		COALESCE(SUM(status = 'a_liberar'), 0) AS a_liberar,
		COALESCE(SUM(status = 'retido'), 0) AS retidos,
		MIN(liberar_em) AS liberar_em
		FROM pagamentos_pendentes WHERE id_desafio = ? AND status IN ('a_liberar','retido')`, idDesafio).Scan(&l).Error
	return l, err
}
