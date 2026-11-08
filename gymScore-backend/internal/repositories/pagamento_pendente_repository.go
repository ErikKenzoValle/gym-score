package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FiltroPendentes parametriza a listagem de pagamentos pendentes (página >= 1, limite 1..100).
type FiltroPendentes struct {
	Status    []string
	IDDesafio *uint
	IDUsuario *uint
	Tipo      string
	Pagina    int
	Limite    int
}

// TotaisPendentes resume o que um usuário tem a receber do pódio (fora do saldo).
type TotaisPendentes struct {
	ALiberar int64
	Retido   int64
}

// PagamentoPendenteRepository acessa os créditos do pódio na janela de contestação. Estados:
// a_liberar -> liberado (job) | retido (denúncia) | cancelado (anulação); retido -> a_liberar | cancelado.
// Toda transição é UPDATE condicional pelo status de origem (idempotente; false/0 = nada mudou).
type PagamentoPendenteRepository interface {
	// CriarLote insere os pendentes do lançamento do pódio (mesma transação do CAS do desafio).
	CriarLote(tx *gorm.DB, lote []models.PagamentoPendente) error
	// TravarDoDesafio devolve todos os pendentes do desafio com FOR UPDATE (anulação, retenção).
	TravarDoDesafio(tx *gorm.DB, idDesafio uint) ([]models.PagamentoPendente, error)
	// Reter: a_liberar -> retido com a denúncia que reteve. Devolve quantos mudaram.
	Reter(tx *gorm.DB, idDesafio, idDenuncia uint) (int64, error)
	// TrocarRetencao repassa os retidos de uma denúncia julgada para outra retentora ainda aberta.
	TrocarRetencao(tx *gorm.DB, idDenunciaAntiga, idDenunciaNova uint) (int64, error)
	// Reabrir: retido (pela denúncia) -> a_liberar, limpando id_denuncia_retencao.
	Reabrir(tx *gorm.DB, idDenuncia uint) (int64, error)
	// DesafiosProntos devolve desafios com pendentes a_liberar e liberar_em <= agora (job; sem trava).
	DesafiosProntos(agora time.Time, limite int) ([]uint, error)
	// TravarLiberaveis trava (FOR UPDATE SKIP LOCKED) os a_liberar vencidos do desafio, em ordem estável
	// (id_usuario, id) para não inverter a ordem de travas de saldo entre instâncias.
	TravarLiberaveis(tx *gorm.DB, idDesafio uint, agora time.Time) ([]models.PagamentoPendente, error)
	// MarcarLiberado: a_liberar -> liberado com liberado_em e a linha do ledger (nil na plataforma).
	MarcarLiberado(tx *gorm.DB, id uint, idTransacao *uint, em time.Time) (bool, error)
	// CancelarDoDesafio: a_liberar|retido -> cancelado (anulação). Devolve quantos mudaram.
	CancelarDoDesafio(tx *gorm.DB, idDesafio uint) (int64, error)
	// ExisteLiberado diz se algum pendente do desafio já foi liberado (Q3: relançamento proibido).
	ExisteLiberado(tx *gorm.DB, idDesafio uint) (bool, error)
	// TotaisDoUsuario soma a_liberar e retido do usuário (carteira: "a liberar").
	TotaisDoUsuario(idUsuario uint) (TotaisPendentes, error)
	// DoUsuario lista os pendentes não cancelados do usuário (mais recentes primeiro).
	DoUsuario(idUsuario uint, pagina, limite int) ([]models.PagamentoPendente, int64, error)
	Listar(f FiltroPendentes) ([]models.PagamentoPendente, int64, error)
}

type pagamentoPendenteRepository struct{ db *gorm.DB }

// NovoPagamentoPendenteRepository cria o repositório de pagamentos pendentes.
func NovoPagamentoPendenteRepository(db *gorm.DB) PagamentoPendenteRepository {
	return &pagamentoPendenteRepository{db}
}

func (r *pagamentoPendenteRepository) CriarLote(tx *gorm.DB, lote []models.PagamentoPendente) error {
	if len(lote) == 0 {
		return nil
	}
	return dbOuTx(r.db, tx).Create(&lote).Error
}

func (r *pagamentoPendenteRepository) TravarDoDesafio(tx *gorm.DB, idDesafio uint) ([]models.PagamentoPendente, error) {
	lista := []models.PagamentoPendente{}
	err := dbOuTx(r.db, tx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id_desafio = ?", idDesafio).Order("id ASC").Find(&lista).Error
	return lista, err
}

func afetadas(res *gorm.DB) (int64, error) { return res.RowsAffected, res.Error }

func (r *pagamentoPendenteRepository) Reter(tx *gorm.DB, idDesafio, idDenuncia uint) (int64, error) {
	return afetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE pagamentos_pendentes SET status = ?, id_denuncia_retencao = ? WHERE id_desafio = ? AND status = ?",
		models.PendenteRetido, idDenuncia, idDesafio, models.PendenteALiberar))
}

func (r *pagamentoPendenteRepository) TrocarRetencao(tx *gorm.DB, idDenunciaAntiga, idDenunciaNova uint) (int64, error) {
	return afetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE pagamentos_pendentes SET id_denuncia_retencao = ? WHERE id_denuncia_retencao = ? AND status = ?",
		idDenunciaNova, idDenunciaAntiga, models.PendenteRetido))
}

func (r *pagamentoPendenteRepository) Reabrir(tx *gorm.DB, idDenuncia uint) (int64, error) {
	return afetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE pagamentos_pendentes SET status = ?, id_denuncia_retencao = NULL WHERE id_denuncia_retencao = ? AND status = ?",
		models.PendenteALiberar, idDenuncia, models.PendenteRetido))
}

func (r *pagamentoPendenteRepository) DesafiosProntos(agora time.Time, limite int) ([]uint, error) {
	if limite < 1 {
		limite = 50
	}
	var ids []uint
	err := r.db.Model(&models.PagamentoPendente{}).Distinct("id_desafio").
		Where("status = ? AND liberar_em <= ?", models.PendenteALiberar, agora).
		Order("id_desafio ASC").Limit(limite).Pluck("id_desafio", &ids).Error
	return ids, err
}

func (r *pagamentoPendenteRepository) TravarLiberaveis(tx *gorm.DB, idDesafio uint, agora time.Time) ([]models.PagamentoPendente, error) {
	lista := []models.PagamentoPendente{}
	err := dbOuTx(r.db, tx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("id_desafio = ? AND status = ? AND liberar_em <= ?", idDesafio, models.PendenteALiberar, agora).
		Order("id_usuario ASC, id ASC").Find(&lista).Error
	return lista, err
}

func (r *pagamentoPendenteRepository) MarcarLiberado(tx *gorm.DB, id uint, idTransacao *uint, em time.Time) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE pagamentos_pendentes SET status = ?, liberado_em = ?, id_transacao = ? WHERE id = ? AND status = ?",
		models.PendenteLiberado, em, idTransacao, id, models.PendenteALiberar))
}

func (r *pagamentoPendenteRepository) CancelarDoDesafio(tx *gorm.DB, idDesafio uint) (int64, error) {
	return afetadas(dbOuTx(r.db, tx).Exec(
		"UPDATE pagamentos_pendentes SET status = ? WHERE id_desafio = ? AND status IN ?",
		models.PendenteCancelado, idDesafio, []string{models.PendenteALiberar, models.PendenteRetido}))
}

func (r *pagamentoPendenteRepository) ExisteLiberado(tx *gorm.DB, idDesafio uint) (bool, error) {
	var n int64
	err := dbOuTx(r.db, tx).Model(&models.PagamentoPendente{}).
		Where("id_desafio = ? AND status = ?", idDesafio, models.PendenteLiberado).Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *pagamentoPendenteRepository) TotaisDoUsuario(idUsuario uint) (TotaisPendentes, error) {
	var t TotaisPendentes
	err := r.db.Raw(`SELECT
		COALESCE(SUM(CASE WHEN status = 'a_liberar' THEN valor_centavos END), 0) AS a_liberar,
		COALESCE(SUM(CASE WHEN status = 'retido' THEN valor_centavos END), 0) AS retido
		FROM pagamentos_pendentes WHERE id_usuario = ? AND status IN ('a_liberar','retido')`, idUsuario).Scan(&t).Error
	return t, err
}

func (r *pagamentoPendenteRepository) DoUsuario(idUsuario uint, pagina, limite int) ([]models.PagamentoPendente, int64, error) {
	q := r.db.Model(&models.PagamentoPendente{}).Where("id_usuario = ? AND status <> ?", idUsuario, models.PendenteCancelado)
	return paginarContando[models.PagamentoPendente](q, pagina, limite, "id DESC", nil)
}

func (r *pagamentoPendenteRepository) Listar(f FiltroPendentes) ([]models.PagamentoPendente, int64, error) {
	q := r.db.Model(&models.PagamentoPendente{})
	if len(f.Status) > 0 {
		q = q.Where("status IN ?", f.Status)
	}
	if f.IDDesafio != nil {
		q = q.Where("id_desafio = ?", *f.IDDesafio)
	}
	if f.IDUsuario != nil {
		q = q.Where("id_usuario = ?", *f.IDUsuario)
	}
	if f.Tipo != "" {
		q = q.Where("tipo = ?", f.Tipo)
	}
	return paginarContando[models.PagamentoPendente](q, f.Pagina, f.Limite, "liberar_em ASC, id ASC", nil)
}
