package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FiltroDenuncias parametriza as filas/listagens de denúncias (página >= 1, limite 1..100).
// IDAcademia filtra por id_academia_julgadora (fila da academia); vazio/nil = sem filtro.
type FiltroDenuncias struct {
	Status        []string
	Jurisdicao    string
	Categoria     string
	IDAcademia    *uint
	IDDenunciante *uint
	IDDenunciado  *uint
	IDDesafio     *uint
	Pagina        int
	Limite        int
}

// DenunciaRepository acessa denúncias e evidências. Nada é apagado; toda transição de estado é um
// UPDATE condicional único (CAS) que devolve false quando a denúncia não estava no estado esperado.
// Os métodos com `tx` rodam na transação do chamador (nil = conexão do repositório).
type DenunciaRepository interface {
	// Criar insere a denúncia (status/jurisdição preenchidos pelo serviço). 1062 (EhDuplicata) =
	// já existe denúncia aberta para (denunciante, denunciado, desafio) — uq_denuncia_aberta.
	Criar(tx *gorm.DB, d *models.Denuncia) error
	// Buscar devolve a denúncia com as evidências (nil se não existir). travar = SELECT ... FOR UPDATE.
	Buscar(tx *gorm.DB, id uint, travar bool) (*models.Denuncia, error)
	// ContarDoDenuncianteDesde conta denúncias abertas pelo usuário desde `desde` (limite por dia).
	ContarDoDenuncianteDesde(idDenunciante uint, desde time.Time) (int64, error)
	// AdicionarEvidencia grava uma imagem. 1062 = ordem já usada nessa denúncia (uq_evidencia_ordem).
	AdicionarEvidencia(tx *gorm.DB, e *models.DenunciaEvidencia) error
	// ProximaOrdemEvidencia devolve a próxima ordem livre (1..3) travando as evidências da denúncia;
	// 0 = já tem 3.
	ProximaOrdemEvidencia(tx *gorm.DB, idDenuncia uint) (uint8, error)
	// AssumirComoAdmin: aberta|em_analise (jurisdição admin) -> em_analise com id_responsavel_admin.
	AssumirComoAdmin(tx *gorm.DB, id, idAdmin uint) (bool, error)
	// AssumirComoAcademia: aberta (jurisdição academia da própria academia) -> em_analise com id_responsavel.
	AssumirComoAcademia(tx *gorm.DB, id, idAcademia, idUsuarioAcademia uint) (bool, error)
	// Encaminhar (academia -> admin) ou escalonar (job): jurisdicao = admin, status = aberta, sem
	// responsável, encaminhada_em = em. id_academia_julgadora é mantido. idAcademia nil = qualquer (job).
	Encaminhar(tx *gorm.DB, id uint, idAcademia *uint, em time.Time) (bool, error)
	// EscalonaveisVencidas devolve ids de denúncias da academia com prazo vencido (job; SKIP LOCKED).
	EscalonaveisVencidas(tx *gorm.DB, agora time.Time, limite int) ([]uint, error)
	// Decidir grava a decisão final (procedente | improcedente | arquivada) a partir de aberta|em_analise,
	// exigindo a jurisdição informada (e a academia julgadora quando jurisdicao = academia).
	Decidir(tx *gorm.DB, id uint, status, texto string, em time.Time, jurisdicao string, idAcademia *uint) (bool, error)
	// RetentoraAberta devolve a denúncia aberta mais antiga do desafio que retém pagamentos (categorias
	// de desafio), ignorando `exceto` (0 = nenhuma). nil se não houver.
	RetentoraAberta(tx *gorm.DB, idDesafio, exceto uint) (*uint, error)
	// Listar devolve uma página ordenada por criado_em (mais antigas primeiro: é fila).
	Listar(f FiltroDenuncias) ([]models.Denuncia, int64, error)
}

type denunciaRepository struct{ db *gorm.DB }

// NovoDenunciaRepository cria o repositório de denúncias.
func NovoDenunciaRepository(db *gorm.DB) DenunciaRepository { return &denunciaRepository{db} }

var statusDenunciaAberta = []string{models.DenunciaAberta, models.DenunciaEmAnalise}

func (r *denunciaRepository) Criar(tx *gorm.DB, d *models.Denuncia) error {
	return dbOuTx(r.db, tx).Omit("ChaveAberta", "Evidencias").Create(d).Error
}

func (r *denunciaRepository) Buscar(tx *gorm.DB, id uint, travar bool) (*models.Denuncia, error) {
	q := dbOuTx(r.db, tx)
	if travar {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var d models.Denuncia
	err := q.Preload("Evidencias", func(db *gorm.DB) *gorm.DB { return db.Order("ordem ASC") }).First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

func (r *denunciaRepository) ContarDoDenuncianteDesde(idDenunciante uint, desde time.Time) (int64, error) {
	var n int64
	err := r.db.Model(&models.Denuncia{}).Where("id_denunciante = ? AND criado_em >= ?", idDenunciante, desde).Count(&n).Error
	return n, err
}

func (r *denunciaRepository) AdicionarEvidencia(tx *gorm.DB, e *models.DenunciaEvidencia) error {
	return dbOuTx(r.db, tx).Create(e).Error
}

func (r *denunciaRepository) ProximaOrdemEvidencia(tx *gorm.DB, idDenuncia uint) (uint8, error) {
	var ordens []uint8
	if err := dbOuTx(r.db, tx).Model(&models.DenunciaEvidencia{}).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id_denuncia = ?", idDenuncia).Pluck("ordem", &ordens).Error; err != nil {
		return 0, err
	}
	usadas := map[uint8]bool{}
	for _, o := range ordens {
		usadas[o] = true
	}
	for o := uint8(1); o <= models.DenunciaEvidenciasMaximo; o++ {
		if !usadas[o] {
			return o, nil
		}
	}
	return 0, nil
}

func (r *denunciaRepository) AssumirComoAdmin(tx *gorm.DB, id, idAdmin uint) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		`UPDATE denuncias SET status = ?, id_responsavel = NULL, id_responsavel_admin = ?
		  WHERE id = ? AND jurisdicao = ? AND status IN ?`,
		models.DenunciaEmAnalise, idAdmin, id, models.JurisdicaoAdmin, statusDenunciaAberta))
}

func (r *denunciaRepository) AssumirComoAcademia(tx *gorm.DB, id, idAcademia, idUsuarioAcademia uint) (bool, error) {
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(
		`UPDATE denuncias SET status = ?, id_responsavel = ?, id_responsavel_admin = NULL
		  WHERE id = ? AND jurisdicao = ? AND id_academia_julgadora = ? AND status = ?`,
		models.DenunciaEmAnalise, idUsuarioAcademia, id, models.JurisdicaoAcademia, idAcademia, models.DenunciaAberta))
}

func (r *denunciaRepository) Encaminhar(tx *gorm.DB, id uint, idAcademia *uint, em time.Time) (bool, error) {
	q := `UPDATE denuncias SET jurisdicao = ?, status = ?, id_responsavel = NULL, id_responsavel_admin = NULL, encaminhada_em = ?
	       WHERE id = ? AND jurisdicao = ? AND status IN ?`
	args := []any{models.JurisdicaoAdmin, models.DenunciaAberta, em, id, models.JurisdicaoAcademia, statusDenunciaAberta}
	if idAcademia != nil {
		q += " AND id_academia_julgadora = ?"
		args = append(args, *idAcademia)
	}
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(q, args...))
}

func (r *denunciaRepository) EscalonaveisVencidas(tx *gorm.DB, agora time.Time, limite int) ([]uint, error) {
	if limite < 1 {
		limite = 100
	}
	var ids []uint
	err := dbOuTx(r.db, tx).Model(&models.Denuncia{}).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status IN ? AND jurisdicao = ? AND prazo_academia_ate <= ?", statusDenunciaAberta, models.JurisdicaoAcademia, agora).
		Order("prazo_academia_ate ASC, id ASC").Limit(limite).Pluck("id", &ids).Error
	return ids, err
}

func (r *denunciaRepository) Decidir(tx *gorm.DB, id uint, status, texto string, em time.Time, jurisdicao string, idAcademia *uint) (bool, error) {
	switch status {
	case models.DenunciaProcedente, models.DenunciaImprocedente, models.DenunciaArquivada:
	default:
		return false, gorm.ErrInvalidValue
	}
	q := `UPDATE denuncias SET status = ?, decisao_texto = ?, decidida_em = ? WHERE id = ? AND status IN ? AND jurisdicao = ?`
	args := []any{status, texto, em, id, statusDenunciaAberta, jurisdicao}
	if jurisdicao == models.JurisdicaoAcademia {
		if idAcademia == nil {
			return false, gorm.ErrInvalidValue
		}
		q += " AND id_academia_julgadora = ?"
		args = append(args, *idAcademia)
	}
	return linhasAfetadas(dbOuTx(r.db, tx).Exec(q, args...))
}

func (r *denunciaRepository) RetentoraAberta(tx *gorm.DB, idDesafio, exceto uint) (*uint, error) {
	var ids []uint
	err := dbOuTx(r.db, tx).Model(&models.Denuncia{}).
		Where("id_desafio = ? AND status IN ? AND categoria IN ? AND id <> ?", idDesafio, statusDenunciaAberta,
			[]string{models.DenunciaSubornoInstrutor, models.DenunciaManipulacaoPodio, models.DenunciaFraudePresenca}, exceto).
		Order("id ASC").Limit(1).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	return &ids[0], nil
}

func (r *denunciaRepository) Listar(f FiltroDenuncias) ([]models.Denuncia, int64, error) {
	q := r.db.Model(&models.Denuncia{})
	if len(f.Status) > 0 {
		q = q.Where("status IN ?", f.Status)
	}
	if f.Jurisdicao != "" {
		q = q.Where("jurisdicao = ?", f.Jurisdicao)
	}
	if f.Categoria != "" {
		q = q.Where("categoria = ?", f.Categoria)
	}
	if f.IDAcademia != nil {
		q = q.Where("id_academia_julgadora = ?", *f.IDAcademia)
	}
	if f.IDDenunciante != nil {
		q = q.Where("id_denunciante = ?", *f.IDDenunciante)
	}
	if f.IDDenunciado != nil {
		q = q.Where("id_denunciado = ?", *f.IDDenunciado)
	}
	if f.IDDesafio != nil {
		q = q.Where("id_desafio = ?", *f.IDDesafio)
	}
	// Evidências (só metadados) vêm junto: as listas mostram quantas/quais imagens há sem uma
	// consulta por denúncia.
	return paginarContando[models.Denuncia](q, f.Pagina, f.Limite, "criado_em ASC, id ASC", func(db *gorm.DB) *gorm.DB {
		return db.Preload("Evidencias", func(db *gorm.DB) *gorm.DB { return db.Order("ordem ASC") })
	})
}
