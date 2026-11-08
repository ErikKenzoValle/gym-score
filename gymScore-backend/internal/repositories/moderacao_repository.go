package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// ModeracaoRepository concentra as escritas de moderação do painel admin (aprovação de
// academia/instrutor e marca de revisão de saque). O bloqueio de usuário virou punição
// (PunicaoRepository + RecalcularEstadoConta). Todo método é um UPDATE condicional
// único (idempotente: false = nada mudou) e recebe `tx` (pode ser nil) para rodar na MESMA transação
// da linha de admin_auditoria.
type ModeracaoRepository interface {
	// DecidirAcademia / DecidirInstrutor mudam status de 'pendente' para 'aprovada' ou 'rejeitada'
	// (motivo só na rejeição), gravando aprovado_por/aprovado_em. false se não estava pendente.
	DecidirAcademia(tx *gorm.DB, id uint, status string, motivo *string, idAdmin uint, em time.Time) (bool, error)
	DecidirInstrutor(tx *gorm.DB, id uint, status string, motivo *string, idAdmin uint, em time.Time) (bool, error)
	// MarcarSaqueEmRevisao marca um saque `pending` para revisão manual. false se não é saque pendente
	// ou já estava marcado.
	MarcarSaqueEmRevisao(tx *gorm.DB, id uint, motivo string) (bool, error)
}

type moderacaoRepository struct{ db *gorm.DB }

// NovoModeracaoRepository cria uma nova instância do repositório de moderação
func NovoModeracaoRepository(db *gorm.DB) ModeracaoRepository { return &moderacaoRepository{db} }

func linhasAfetadas(res *gorm.DB) (bool, error) { return res.RowsAffected > 0, res.Error }

func (r *moderacaoRepository) decidir(tx *gorm.DB, tabela string, id uint, status string, motivo *string, idAdmin uint, em time.Time) (bool, error) {
	if status != models.CadastroAprovado && status != models.CadastroRejeitado {
		return false, gorm.ErrInvalidValue
	}
	if status == models.CadastroAprovado {
		motivo = nil
	}
	// tabela é sempre uma constante interna (academias | instrutores), nunca entrada do usuário.
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE "+tabela+" SET status = ?, motivo_rejeicao = ?, aprovado_por = ?, aprovado_em = ? WHERE id = ? AND status = ?",
		status, motivo, idAdmin, em, id, models.CadastroPendente))
}

func (r *moderacaoRepository) DecidirAcademia(tx *gorm.DB, id uint, status string, motivo *string, idAdmin uint, em time.Time) (bool, error) {
	return r.decidir(tx, "academias", id, status, motivo, idAdmin, em)
}

func (r *moderacaoRepository) DecidirInstrutor(tx *gorm.DB, id uint, status string, motivo *string, idAdmin uint, em time.Time) (bool, error) {
	return r.decidir(tx, "instrutores", id, status, motivo, idAdmin, em)
}

func (r *moderacaoRepository) MarcarSaqueEmRevisao(tx *gorm.DB, id uint, motivo string) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE transacoes SET em_revisao = 1, revisao_motivo = ? WHERE id = ? AND tipo = ? AND status = ? AND em_revisao = 0",
		motivo, id, models.TipoSaque, models.TransacaoPending))
}
