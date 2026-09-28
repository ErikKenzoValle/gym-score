package repositories

import (
	"errors"

	"gynScore-backend/internal/models"
	"gorm.io/gorm"
)

// DesafioRepository define a interface de acesso a dados para desafios
type DesafioRepository interface {
	Criar(desafio *models.Desafio) error
	BuscarPorID(id uint) (*models.Desafio, error)
	Listar() ([]models.Desafio, error)
	ListarPorUsuario(idUsuario uint) ([]models.Desafio, error)
	Atualizar(desafio *models.Desafio) error
	Deletar(id uint) error

	// Participantes
	AdicionarParticipante(p *models.DesafioParticipante) error
	ContarParticipantes(idDesafio uint) (int64, error)
	UsuarioParticipa(idDesafio, idUsuario uint) (bool, error)
	ListarParticipantes(idDesafio uint) ([]models.DesafioParticipante, error)

	// Histórico e presença
	ListarHistoricoUsuario(idUsuario uint) ([]models.Desafio, error)
	MarcarPresente(idDesafio, idUsuario uint) error
	ListarPresentes(idDesafio uint) ([]models.DesafioParticipante, error)

	// Ingressos — retorna todos os desafios em que o usuário é participante
	ListarParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error)
}

// desafioRepository é a implementação concreta usando GORM
type desafioRepository struct {
	db *gorm.DB
}

// NovoDesafioRepository cria uma nova instância do repositório de desafios
func NovoDesafioRepository(db *gorm.DB) DesafioRepository {
	return &desafioRepository{db: db}
}

// Criar insere um novo desafio no banco de dados
func (r *desafioRepository) Criar(desafio *models.Desafio) error {
	return r.db.Create(desafio).Error
}

// BuscarPorID retorna um desafio pelo seu ID com os relacionamentos carregados
func (r *desafioRepository) BuscarPorID(id uint) (*models.Desafio, error) {
	var desafio models.Desafio
	err := r.db.Preload("Criador").Preload("Desafiado").
		Preload("Participantes.Usuario").First(&desafio, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &desafio, nil
}

// Listar retorna todos os desafios cadastrados
func (r *desafioRepository) Listar() ([]models.Desafio, error) {
	var desafios []models.Desafio
	err := r.db.Preload("Criador").Preload("Desafiado").
		Preload("Participantes.Usuario").Find(&desafios).Error
	return desafios, err
}

// ListarPorUsuario retorna os desafios ativos que o usuário criou ou participa
func (r *desafioRepository) ListarPorUsuario(idUsuario uint) ([]models.Desafio, error) {
	var desafios []models.Desafio
	participaSub := r.db.Model(&models.DesafioParticipante{}).
		Select("id_desafio").Where("id_usuario = ?", idUsuario)
	err := r.db.Preload("Criador").Preload("Participantes.Usuario").
		Where("(id_criador = ? OR id IN (?)) AND status IN ?",
			idUsuario, participaSub,
			[]models.StatusDesafio{models.StatusAberto, models.StatusPendente, models.StatusEmAndamento}).
		Find(&desafios).Error
	return desafios, err
}

// Atualizar salva as alterações de um desafio existente
func (r *desafioRepository) Atualizar(desafio *models.Desafio) error {
	return r.db.Save(desafio).Error
}

// Deletar remove um desafio pelo seu ID
func (r *desafioRepository) Deletar(id uint) error {
	return r.db.Delete(&models.Desafio{}, id).Error
}

// AdicionarParticipante inscreve um usuário em um desafio
func (r *desafioRepository) AdicionarParticipante(p *models.DesafioParticipante) error {
	return r.db.Create(p).Error
}

// ContarParticipantes retorna quantos usuários estão inscritos no desafio
func (r *desafioRepository) ContarParticipantes(idDesafio uint) (int64, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ?", idDesafio).Count(&total).Error
	return total, err
}

// UsuarioParticipa indica se o usuário já está inscrito no desafio
func (r *desafioRepository) UsuarioParticipa(idDesafio, idUsuario uint) (bool, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Count(&total).Error
	return total > 0, err
}

// ListarParticipantes retorna os participantes de um desafio com seus usuários
func (r *desafioRepository) ListarParticipantes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var participantes []models.DesafioParticipante
	err := r.db.Preload("Usuario").
		Where("id_desafio = ?", idDesafio).Find(&participantes).Error
	return participantes, err
}

// ListarHistoricoUsuario retorna os desafios encerrados onde o usuário criou ou participou
func (r *desafioRepository) ListarHistoricoUsuario(idUsuario uint) ([]models.Desafio, error) {
	var desafios []models.Desafio
	participaSub := r.db.Model(&models.DesafioParticipante{}).
		Select("id_desafio").Where("id_usuario = ?", idUsuario)
	err := r.db.Preload("Criador").
		Where("status = ? AND (id_criador = ? OR id IN (?))",
			models.StatusEncerrado, idUsuario, participaSub).
		Order("criado_em DESC").
		Find(&desafios).Error
	return desafios, err
}

// MarcarPresente atualiza o campo presente do participante para true
func (r *desafioRepository) MarcarPresente(idDesafio, idUsuario uint) error {
	return r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Update("presente", true).Error
}

// ListarPresentes retorna os participantes marcados como presentes em um desafio
func (r *desafioRepository) ListarPresentes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var participantes []models.DesafioParticipante
	return participantes, r.db.Preload("Usuario").
		Where("id_desafio = ? AND presente = true", idDesafio).
		Find(&participantes).Error
}

// ListarParticipacoesUsuario retorna todos os desafios nos quais o usuário é participante,
// fazendo JOIN na tabela desafio_participantes
func (r *desafioRepository) ListarParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error) {
	var desafios []models.Desafio
	err := r.db.
		Joins("JOIN desafio_participantes dp ON dp.id_desafio = desafios.id").
		Where("dp.id_usuario = ?", idUsuario).
		Find(&desafios).Error
	return desafios, err
}
