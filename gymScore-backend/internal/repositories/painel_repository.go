package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// PainelRepository reúne consultas de leitura agregadas, escopadas por academia ou por usuário:
// KPIs do dashboard da academia, ingressos e histórico do atleta. Não grava nada.
type PainelRepository struct {
	db *gorm.DB
}

// NovoPainelRepository cria o repositório de consultas agregadas.
func NovoPainelRepository(db *gorm.DB) *PainelRepository {
	return &PainelRepository{db: db}
}

// ContagemPorStatus conta os desafios da academia por status (só os status presentes no resultado).
func (r *PainelRepository) ContagemPorStatus(idAcademia uint) (map[models.StatusDesafio]int64, error) {
	var linhas []struct {
		Status models.StatusDesafio
		Total  int64
	}
	err := r.db.Model(&models.Desafio{}).Select("status, COUNT(*) AS total").
		Where("id_academia = ?", idAcademia).Group("status").Scan(&linhas).Error
	if err != nil {
		return nil, err
	}
	out := make(map[models.StatusDesafio]int64, len(linhas))
	for _, l := range linhas {
		out[l.Status] = l.Total
	}
	return out, nil
}

// InscritosDesde conta as inscrições feitas a partir de `desde` em desafios não cancelados da academia.
func (r *PainelRepository) InscritosDesde(idAcademia uint, desde time.Time) (int64, error) {
	var total int64
	err := r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Where("d.id_academia = ? AND d.status <> ? AND p.criado_em >= ?", idAcademia, models.StatusCancelado, desde).
		Count(&total).Error
	return total, err
}

// PoteEmJogo soma as entradas dos inscritos nos desafios abertos e em andamento da academia (centavos).
func (r *PainelRepository) PoteEmJogo(idAcademia uint) (int64, error) {
	var total int64
	err := r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Select("COALESCE(SUM(d.valor_entrada_centavos), 0)").
		Where("d.id_academia = ? AND d.status IN ?", idAcademia,
			[]models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}).
		Scan(&total).Error
	return total, err
}

// Presenca devolve (presentes, inscritos) dos desafios em andamento e encerrados da academia.
func (r *PainelRepository) Presenca(idAcademia uint) (presentes, inscritos int64, err error) {
	var l struct{ Presentes, Inscritos int64 }
	err = r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Select("COALESCE(SUM(p.presente), 0) AS presentes, COUNT(*) AS inscritos").
		Where("d.id_academia = ? AND d.status IN ?", idAcademia,
			[]models.StatusDesafio{models.StatusEmAndamento, models.StatusEncerrado}).
		Scan(&l).Error
	return l.Presentes, l.Inscritos, err
}

// AvaliacaoMedia devolve a média e a quantidade de avaliações dos desafios da academia.
func (r *PainelRepository) AvaliacaoMedia(idAcademia uint) (media float64, total int64, err error) {
	var l struct {
		Media float64
		Total int64
	}
	err = r.db.Table("avaliacoes_desafios a").
		Joins("JOIN desafios d ON d.id = a.id_desafio").
		Select("COALESCE(AVG(a.nota), 0) AS media, COUNT(*) AS total").
		Where("d.id_academia = ?", idAcademia).Scan(&l).Error
	return l.Media, l.Total, err
}
