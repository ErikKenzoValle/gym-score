package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrVotoDuplicado indica que o eleitor já votou neste desafio.
var ErrVotoDuplicado = errors.New("voto já registrado")

// FiltroDesafios parametriza a listagem (paginação e filtro feitos no SQL).
type FiltroDesafios struct {
	IDUsuario   uint
	Status      models.StatusDesafio // vazio = sem filtro
	SomenteMeus bool                 // true: apenas desafios em que o usuário participa/criou
	// Todos ignora o filtro de visibilidade (uso administrativo: dashboard de academia, agenda de instrutor).
	Todos  bool
	Pagina int
	Limite int
}

// DesafioRepository acesso a dados de desafios, participantes e votos.
// Os métodos operam sobre r.db; dentro de uma transação use ComTx(tx).
type DesafioRepository struct {
	db *gorm.DB
}

// NovoDesafioRepository cria uma nova instância do repositório de desafios
func NovoDesafioRepository(db *gorm.DB) *DesafioRepository {
	return &DesafioRepository{db: db}
}

// ComTx devolve um repositório que opera dentro da transação informada.
func (r *DesafioRepository) ComTx(tx *gorm.DB) *DesafioRepository {
	return &DesafioRepository{db: tx}
}

// Criar insere um novo desafio no banco de dados
func (r *DesafioRepository) Criar(d *models.Desafio) error {
	return r.db.Create(d).Error
}

// TravarPorID lê o desafio com SELECT ... FOR UPDATE (serializa operações concorrentes
// sobre o mesmo desafio). Só faz sentido dentro de uma transação. Retorna nil se não existir.
func (r *DesafioRepository) TravarPorID(id uint) (*models.Desafio, error) {
	var d models.Desafio
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

// BuscarDetalhe retorna o desafio com criador e participantes (ordem de entrada).
func (r *DesafioRepository) BuscarDetalhe(id uint) (*models.Desafio, error) {
	var d models.Desafio
	err := r.db.Preload("Criador").
		Preload("Participantes", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Participantes.Usuario").
		First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

// MudarStatus faz a transição idempotente: só muda se o status atual for `de`.
// Retorna false (sem erro) quando outra chamada já mudou o status.
func (r *DesafioRepository) MudarStatus(id uint, de, para models.StatusDesafio, extras map[string]any) (bool, error) {
	campos := map[string]any{"status": para}
	for k, v := range extras {
		campos[k] = v
	}
	res := r.db.Model(&models.Desafio{}).
		Where("id = ? AND status = ?", id, de).
		Updates(campos)
	return res.RowsAffected > 0, res.Error
}

const visivelSQL = `((status = 'aberto' AND (SELECT COUNT(*) FROM desafio_participantes p WHERE p.id_desafio = desafios.id) < vagas)
	OR id_criador = ?
	OR EXISTS (SELECT 1 FROM desafio_participantes p WHERE p.id_desafio = desafios.id AND p.id_usuario = ?))`

const meuSQL = `(id_criador = ? OR EXISTS (SELECT 1 FROM desafio_participantes p WHERE p.id_desafio = desafios.id AND p.id_usuario = ?))`

// Listar retorna uma página de desafios e o total (filtros e paginação no SQL).
func (r *DesafioRepository) Listar(f FiltroDesafios) ([]models.Desafio, int64, error) {
	q := r.db.Model(&models.Desafio{})
	switch {
	case f.Todos:
		// Sem filtro de visibilidade: uso administrativo (dashboard de academia, agenda de instrutor).
	case f.SomenteMeus:
		q = q.Where(meuSQL, f.IDUsuario, f.IDUsuario)
	default:
		q = q.Where(visivelSQL, f.IDUsuario, f.IDUsuario)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var desafios []models.Desafio
	err := q.Preload("Criador").
		Preload("Participantes", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Participantes.Usuario").
		Order("desafios.criado_em DESC, desafios.id DESC").
		Offset((f.Pagina - 1) * f.Limite).Limit(f.Limite).
		Find(&desafios).Error
	return desafios, total, err
}

// ─── Participantes ───────────────────────────────────────────────────────────

// AdicionarParticipante inscreve um usuário em um desafio
func (r *DesafioRepository) AdicionarParticipante(p *models.DesafioParticipante) error {
	return r.db.Create(p).Error
}

// RemoverParticipante tira o usuário do desafio; false se ele não estava inscrito.
func (r *DesafioRepository) RemoverParticipante(idDesafio, idUsuario uint) (bool, error) {
	res := r.db.Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Delete(&models.DesafioParticipante{})
	return res.RowsAffected > 0, res.Error
}

// ContarParticipantes retorna quantos usuários estão inscritos no desafio
func (r *DesafioRepository) ContarParticipantes(idDesafio uint) (int64, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ?", idDesafio).Count(&total).Error
	return total, err
}

// UsuarioParticipa indica se o usuário já está inscrito no desafio
func (r *DesafioRepository) UsuarioParticipa(idDesafio, idUsuario uint) (bool, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Count(&total).Error
	return total > 0, err
}

// ListarParticipantes retorna os participantes ordenados por id_usuario (ordem estável p/ apuração).
func (r *DesafioRepository) ListarParticipantes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var ps []models.DesafioParticipante
	err := r.db.Where("id_desafio = ?", idDesafio).Order("id_usuario ASC").Find(&ps).Error
	return ps, err
}

// RegistrarPremio grava o prêmio e a marca de vencedor de um participante.
func (r *DesafioRepository) RegistrarPremio(idDesafio, idUsuario uint, premio int64) error {
	return r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Updates(map[string]any{"premio_centavos": premio, "vencedor": true}).Error
}

// ─── Votos ───────────────────────────────────────────────────────────────────

// InserirVoto registra o voto; ErrVotoDuplicado se o eleitor já votou.
func (r *DesafioRepository) InserirVoto(v *models.DesafioVoto) error {
	if err := r.db.Create(v).Error; err != nil {
		if EhDuplicata(err) {
			return ErrVotoDuplicado
		}
		return err
	}
	return nil
}

// ContarVotos retorna o total de votos do desafio.
func (r *DesafioRepository) ContarVotos(idDesafio uint) (int64, error) {
	var total int64
	err := r.db.Model(&models.DesafioVoto{}).Where("id_desafio = ?", idDesafio).Count(&total).Error
	return total, err
}

// ListarVotos retorna todos os votos do desafio.
func (r *DesafioRepository) ListarVotos(idDesafio uint) ([]models.DesafioVoto, error) {
	var vs []models.DesafioVoto
	err := r.db.Where("id_desafio = ?", idDesafio).Find(&vs).Error
	return vs, err
}

// ─── Check-ins ───────────────────────────────────────────────────────────────

// ErrCheckinDuplicado indica que o participante já fez check-in hoje neste desafio.
var ErrCheckinDuplicado = errors.New("check-in já registrado hoje")

// InserirCheckin grava o check-in; ErrCheckinDuplicado se já houver um no dia.
func (r *DesafioRepository) InserirCheckin(c *models.DesafioCheckin) error {
	if err := r.db.Create(c).Error; err != nil {
		if EhDuplicata(err) {
			return ErrCheckinDuplicado
		}
		return err
	}
	return nil
}

// CheckinNoDia indica se o usuário já fez check-in no dia (YYYY-MM-DD).
func (r *DesafioRepository) CheckinNoDia(idDesafio, idUsuario uint, dia string) (bool, error) {
	var n int64
	err := r.db.Model(&models.DesafioCheckin{}).
		Where("id_desafio = ? AND id_usuario = ? AND data = ?", idDesafio, idUsuario, dia).Count(&n).Error
	return n > 0, err
}

// ListarCheckins devolve os check-ins do desafio (mais recentes primeiro) com o usuário.
func (r *DesafioRepository) ListarCheckins(idDesafio uint) ([]models.DesafioCheckin, error) {
	var cs []models.DesafioCheckin
	err := r.db.Preload("Usuario").Where("id_desafio = ?", idDesafio).
		Order("criado_em DESC, id DESC").Find(&cs).Error
	return cs, err
}

// ResumoCheckin é a contagem e o último check-in de um participante.
type ResumoCheckin struct {
	IDUsuario uint
	Total     int64
	Ultimo    *time.Time
}

// ResumoCheckins agrega os check-ins do desafio por participante.
func (r *DesafioRepository) ResumoCheckins(idDesafio uint) (map[uint]ResumoCheckin, error) {
	var linhas []struct {
		IDUsuario uint
		Total     int64
		Ultimo    *time.Time
	}
	err := r.db.Model(&models.DesafioCheckin{}).
		Select("id_usuario, COUNT(*) AS total, MAX(criado_em) AS ultimo").
		Where("id_desafio = ?", idDesafio).Group("id_usuario").Scan(&linhas).Error
	out := make(map[uint]ResumoCheckin, len(linhas))
	for _, l := range linhas {
		out[l.IDUsuario] = ResumoCheckin{l.IDUsuario, l.Total, l.Ultimo}
	}
	return out, err
}

// ListarHistoricoUsuario retorna os desafios encerrados onde o usuário criou ou participou
// ListarHistoricoUsuario retorna os desafios encerrados em que idUsuario participou (criador
// incluso, pois o criador também é inserido como participante ao criar). A Participantes vem
// filtrada só para a linha de idUsuario — é o resultado (vencedor/prêmio) dele que importa aqui,
// não o dos demais participantes (o histórico é exposto no perfil público de terceiros).
func (r *DesafioRepository) ListarHistoricoUsuario(idUsuario uint) ([]models.Desafio, error) {
	var desafios []models.Desafio
	participaSub := r.db.Model(&models.DesafioParticipante{}).
		Select("id_desafio").Where("id_usuario = ?", idUsuario)
	err := r.db.
		Preload("Participantes", "id_usuario = ?", idUsuario).
		Where("status = ? AND (id_criador = ? OR id IN (?))",
			models.StatusEncerrado, idUsuario, participaSub).
		Order("criado_em DESC").
		Find(&desafios).Error
	return desafios, err
}

// MarcarPresente atualiza o campo presente do participante para true
func (r *DesafioRepository) MarcarPresente(idDesafio, idUsuario uint) error {
	return r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Update("presente", true).Error
}

// ListarPresentes retorna os participantes marcados como presentes em um desafio
func (r *DesafioRepository) ListarPresentes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var participantes []models.DesafioParticipante
	return participantes, r.db.Preload("Usuario").
		Where("id_desafio = ? AND presente = true", idDesafio).
		Find(&participantes).Error
}

// ListarParticipacoesUsuario retorna todos os desafios nos quais o usuário é participante,
// fazendo JOIN na tabela desafio_participantes
func (r *DesafioRepository) ListarParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error) {
	var desafios []models.Desafio
	err := r.db.
		Joins("JOIN desafio_participantes dp ON dp.id_desafio = desafios.id").
		Where("dp.id_usuario = ?", idUsuario).
		Find(&desafios).Error
	return desafios, err
}
