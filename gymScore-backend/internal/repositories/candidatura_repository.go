package repositories

import (
	"errors"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// CandidaturaRepository acessa as candidaturas de instrutores a vagas abertas de desafio.
// Os métodos operam sobre r.db; dentro de uma transação use ComTx(tx). As mudanças de estado são
// feitas com o desafio travado (FOR UPDATE) pelo service, que serializa candidatar/retirar/escolher.
type CandidaturaRepository struct {
	db *gorm.DB
}

// NovoCandidaturaRepository cria o repositório de candidaturas.
func NovoCandidaturaRepository(db *gorm.DB) *CandidaturaRepository {
	return &CandidaturaRepository{db: db}
}

// ComTx devolve um repositório que opera dentro da transação informada.
func (r *CandidaturaRepository) ComTx(tx *gorm.DB) *CandidaturaRepository {
	return &CandidaturaRepository{db: tx}
}

// Buscar devolve a candidatura (com instrutor e usuário) ou nil.
func (r *CandidaturaRepository) Buscar(id uint) (*models.CandidaturaInstrutor, error) {
	var c models.CandidaturaInstrutor
	err := r.db.Preload("Instrutor.Usuario").First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &c, err
}

// BuscarDoInstrutor devolve a candidatura do instrutor neste desafio (qualquer status) ou nil.
func (r *CandidaturaRepository) BuscarDoInstrutor(idDesafio, idInstrutor uint) (*models.CandidaturaInstrutor, error) {
	var c models.CandidaturaInstrutor
	err := r.db.Where("id_desafio = ? AND id_instrutor = ?", idDesafio, idInstrutor).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &c, err
}

// Criar insere uma candidatura nova.
func (r *CandidaturaRepository) Criar(c *models.CandidaturaInstrutor) error {
	return r.db.Create(c).Error
}

// Reabrir reaproveita a linha (UK desafio+instrutor) com novo lance/mensagem e status pendente.
func (r *CandidaturaRepository) Reabrir(id uint, lance int64, mensagem *string) error {
	return r.db.Model(&models.CandidaturaInstrutor{}).Where("id = ?", id).
		Updates(map[string]any{"lance_centavos": lance, "mensagem": mensagem, "status": models.CandidaturaPendente}).Error
}

// MudarStatus faz a transição condicional (de -> para). Retorna false se o status atual não era `de`.
func (r *CandidaturaRepository) MudarStatus(id uint, de, para string) (bool, error) {
	res := r.db.Model(&models.CandidaturaInstrutor{}).Where("id = ? AND status = ?", id, de).Update("status", para)
	return res.RowsAffected > 0, res.Error
}

// RecusarPendentes marca como recusadas todas as pendentes do desafio (exceto `exceto`, 0 = nenhuma
// exceção) e devolve o id de USUÁRIO de cada instrutor afetado, para notificar.
func (r *CandidaturaRepository) RecusarPendentes(idDesafio, exceto uint) ([]uint, error) {
	var cs []models.CandidaturaInstrutor
	q := r.db.Preload("Instrutor").Where("id_desafio = ? AND status = ?", idDesafio, models.CandidaturaPendente)
	if exceto != 0 {
		q = q.Where("id <> ?", exceto)
	}
	if err := q.Find(&cs).Error; err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(cs))
	usuarios := make([]uint, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.ID)
		if c.Instrutor != nil {
			usuarios = append(usuarios, c.Instrutor.IDUsuario)
		}
	}
	err := r.db.Model(&models.CandidaturaInstrutor{}).Where("id IN ?", ids).
		Update("status", models.CandidaturaRecusada).Error
	return usuarios, err
}

// ListarDoDesafio devolve as candidaturas ativas (pendente, escolhida, recusada; sem as retiradas),
// do menor lance para o maior, com instrutor e usuário.
func (r *CandidaturaRepository) ListarDoDesafio(idDesafio uint) ([]models.CandidaturaInstrutor, error) {
	var cs []models.CandidaturaInstrutor
	err := r.db.Preload("Instrutor.Usuario").
		Where("id_desafio = ? AND status <> ?", idDesafio, models.CandidaturaRetirada).
		Order("lance_centavos ASC, criado_em ASC, id ASC").Find(&cs).Error
	return cs, err
}

// MaiorLancePendente devolve o maior lance entre as candidaturas pendentes (0 se não há).
func (r *CandidaturaRepository) MaiorLancePendente(idDesafio uint) (int64, error) {
	var maior int64
	err := r.db.Model(&models.CandidaturaInstrutor{}).Select("COALESCE(MAX(lance_centavos), 0)").
		Where("id_desafio = ? AND status = ?", idDesafio, models.CandidaturaPendente).Scan(&maior).Error
	return maior, err
}

// ContarPendentes devolve, em lote, quantas candidaturas pendentes cada desafio tem.
func (r *CandidaturaRepository) ContarPendentes(ids []uint) (map[uint]int, error) {
	var linhas []struct {
		IDDesafio uint
		Total     int
	}
	err := r.db.Model(&models.CandidaturaInstrutor{}).Select("id_desafio, COUNT(*) AS total").
		Where("id_desafio IN ? AND status = ?", ids, models.CandidaturaPendente).Group("id_desafio").Scan(&linhas).Error
	out := make(map[uint]int, len(linhas))
	for _, l := range linhas {
		out[l.IDDesafio] = l.Total
	}
	return out, err
}

// DoInstrutor devolve, em lote, a candidatura do instrutor em cada desafio de ids.
func (r *CandidaturaRepository) DoInstrutor(idInstrutor uint, ids []uint) (map[uint]models.CandidaturaInstrutor, error) {
	var cs []models.CandidaturaInstrutor
	if err := r.db.Where("id_instrutor = ? AND id_desafio IN ?", idInstrutor, ids).Find(&cs).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]models.CandidaturaInstrutor, len(cs))
	for _, c := range cs {
		out[c.IDDesafio] = c
	}
	return out, nil
}
