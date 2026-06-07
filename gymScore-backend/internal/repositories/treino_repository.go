package repositories

import (
	"errors"

	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// TreinoRepository define o acesso a dados de treinos.
type TreinoRepository interface {
	Criar(treino *models.Treino) error
	BuscarPorID(id uint) (*models.Treino, error)
	BuscarPorCodigo(codigo string) (*models.Treino, error)
	ListarPorUsuario(idUsuario uint) ([]models.Treino, error)
	Deletar(id uint) error
	CodigoExiste(codigo string) (bool, error)

	RegistrarConclusao(c *models.TreinoConclusao) error
	RadarPorUsuario(idUsuario uint) ([]models.RadarGrupo, error)
}

type treinoRepository struct {
	db *gorm.DB
}

func NovoTreinoRepository(db *gorm.DB) TreinoRepository {
	return &treinoRepository{db: db}
}

func (r *treinoRepository) Criar(treino *models.Treino) error {
	return r.db.Create(treino).Error
}

func (r *treinoRepository) BuscarPorID(id uint) (*models.Treino, error) {
	var t models.Treino
	err := r.db.Preload("Exercicios", func(db *gorm.DB) *gorm.DB {
		return db.Order("ordem ASC")
	}).First(&t, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *treinoRepository) BuscarPorCodigo(codigo string) (*models.Treino, error) {
	var t models.Treino
	err := r.db.Preload("Exercicios", func(db *gorm.DB) *gorm.DB {
		return db.Order("ordem ASC")
	}).Where("codigo = ?", codigo).First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func (r *treinoRepository) ListarPorUsuario(idUsuario uint) ([]models.Treino, error) {
	var treinos []models.Treino
	err := r.db.Preload("Exercicios", func(db *gorm.DB) *gorm.DB {
		return db.Order("ordem ASC")
	}).Where("id_usuario = ?", idUsuario).Order("criado_em DESC").Find(&treinos).Error
	return treinos, err
}

func (r *treinoRepository) Deletar(id uint) error {
	// Remove os exercícios e o treino
	if err := r.db.Where("id_treino = ?", id).Delete(&models.TreinoExercicio{}).Error; err != nil {
		return err
	}
	return r.db.Delete(&models.Treino{}, id).Error
}

func (r *treinoRepository) CodigoExiste(codigo string) (bool, error) {
	var total int64
	err := r.db.Model(&models.Treino{}).Where("codigo = ?", codigo).Count(&total).Error
	return total > 0, err
}

func (r *treinoRepository) RegistrarConclusao(c *models.TreinoConclusao) error {
	return r.db.Create(c).Error
}

func (r *treinoRepository) RadarPorUsuario(idUsuario uint) ([]models.RadarGrupo, error) {
	var radar []models.RadarGrupo
	err := r.db.Model(&models.TreinoConclusao{}).
		Select("grupo_muscular, COUNT(*) as total").
		Where("id_usuario = ?", idUsuario).
		Group("grupo_muscular").
		Scan(&radar).Error
	return radar, err
}
