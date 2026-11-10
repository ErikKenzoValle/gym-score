package repositories

import (
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Equipe de instrutores do desafio (tabela desafio_instrutores, migration 00021). Toda mudança na equipe
// roda com o desafio travado (FOR UPDATE) pelo service: é a trava do desafio que garante o tamanho
// máximo (instrutores_vagas) e a "última vaga" — o banco só garante membro único e 1 responsável.

// ordemEquipe: responsável primeiro, depois por ordem de entrada.
const ordemEquipe = "responsavel DESC, criado_em ASC, id ASC"

// preloadEquipe carrega a equipe ordenada, com instrutor e usuário (nome).
func preloadEquipe(db *gorm.DB) *gorm.DB {
	return db.Preload("Equipe", func(q *gorm.DB) *gorm.DB { return q.Order(ordemEquipe) }).
		Preload("Equipe.Instrutor.Usuario")
}

// ListarEquipe devolve os membros da equipe do desafio (com instrutor e usuário), responsável primeiro.
func (r *DesafioRepository) ListarEquipe(idDesafio uint) ([]models.DesafioInstrutor, error) {
	var lista []models.DesafioInstrutor
	err := r.db.Preload("Instrutor.Usuario").Where("id_desafio = ?", idDesafio).Order(ordemEquipe).Find(&lista).Error
	return lista, err
}

// AdicionarMembro insere um membro (1062 = já é da equipe ou segundo responsável).
func (r *DesafioRepository) AdicionarMembro(m *models.DesafioInstrutor) error {
	return r.db.Create(m).Error
}

// RemoverMembro apaga o membro; false se ele não era da equipe. Se era o responsável, o chamador
// precisa atualizar o espelho desafios.id_instrutor (DefinirResponsavel ou LimparResponsavel).
func (r *DesafioRepository) RemoverMembro(idDesafio, idInstrutor uint) (bool, error) {
	res := r.db.Where("id_desafio = ? AND id_instrutor = ?", idDesafio, idInstrutor).Delete(&models.DesafioInstrutor{})
	return res.RowsAffected > 0, res.Error
}

// DefinirResponsavel troca o responsável da equipe: desmarca o atual ANTES de marcar o novo (a UNIQUE da
// coluna gerada responsavel_desafio daria 1062 na ordem inversa) e grava o espelho legado
// desafios.id_instrutor, tudo na transação do chamador (desafio travado). false se o instrutor não é membro.
func (r *DesafioRepository) DefinirResponsavel(idDesafio, idInstrutor uint) (bool, error) {
	if err := r.db.Model(&models.DesafioInstrutor{}).Where("id_desafio = ? AND responsavel = ? AND id_instrutor <> ?",
		idDesafio, true, idInstrutor).Update("responsavel", false).Error; err != nil {
		return false, err
	}
	var n int64
	if err := r.db.Model(&models.DesafioInstrutor{}).Where("id_desafio = ? AND id_instrutor = ?", idDesafio, idInstrutor).
		Count(&n).Error; err != nil || n == 0 {
		return false, err
	}
	if err := r.db.Model(&models.DesafioInstrutor{}).Where("id_desafio = ? AND id_instrutor = ?", idDesafio, idInstrutor).
		Update("responsavel", true).Error; err != nil {
		return false, err
	}
	return true, r.db.Model(&models.Desafio{}).Where("id = ?", idDesafio).Update("id_instrutor", idInstrutor).Error
}

// LimparResponsavel zera o espelho desafios.id_instrutor (equipe sem responsável, ex.: ficou vazia).
func (r *DesafioRepository) LimparResponsavel(idDesafio uint) error {
	return r.db.Model(&models.Desafio{}).Where("id = ?", idDesafio).Update("id_instrutor", nil).Error
}

// ContarEquipes devolve, em lote, o tamanho da equipe de cada desafio.
func (r *DesafioRepository) ContarEquipes(ids []uint) (map[uint]int, error) {
	out := make(map[uint]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var linhas []struct {
		IDDesafio uint
		Total     int
	}
	err := r.db.Model(&models.DesafioInstrutor{}).Select("id_desafio, COUNT(*) AS total").
		Where("id_desafio IN ?", ids).Group("id_desafio").Scan(&linhas).Error
	for _, l := range linhas {
		out[l.IDDesafio] = l.Total
	}
	return out, err
}

// DesafiosDaEquipe devolve os ids dos desafios em que o instrutor é membro da equipe.
func (r *DesafioRepository) DesafiosDaEquipe(idInstrutor uint) ([]uint, error) {
	var ids []uint
	err := r.db.Model(&models.DesafioInstrutor{}).Where("id_instrutor = ?", idInstrutor).Pluck("id_desafio", &ids).Error
	return ids, err
}

// MembroDaEquipe diz se o instrutor é membro da equipe do desafio.
func (r *DesafioRepository) MembroDaEquipe(idDesafio, idInstrutor uint) (bool, error) {
	var n int64
	err := r.db.Model(&models.DesafioInstrutor{}).Where("id_desafio = ? AND id_instrutor = ?", idDesafio, idInstrutor).
		Count(&n).Error
	return n > 0, err
}

// InscricaoNoHorario é uma inscrição ativa do atleta que conflita com o horário de outro desafio.
type InscricaoNoHorario struct {
	IDDesafio     uint
	Titulo        string
	IDAcademia    uint
	DataEvento    time.Time
	DataFimEvento time.Time
}

// InscricoesAtivasComHorario devolve as inscrições do usuário em desafios abertos ou em andamento que
// têm início e fim (legados sem fim nunca chocam), para a regra de choque de horário (T6).
// Leitura consistente: no Entrar ela roda depois da trava da linha do usuário, então enxerga as
// inscrições já confirmadas por transações concorrentes do mesmo atleta.
func (r *DesafioRepository) InscricoesAtivasComHorario(idUsuario uint) ([]InscricaoNoHorario, error) {
	var out []InscricaoNoHorario
	err := r.db.Table("desafio_participantes p").
		Select("d.id AS id_desafio, d.titulo, d.id_academia, d.data_evento, d.data_fim_evento").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Where("p.id_usuario = ? AND d.status IN ? AND d.id_academia IS NOT NULL AND d.data_evento IS NOT NULL AND d.data_fim_evento IS NOT NULL",
			idUsuario, []models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}).
		Order("d.data_evento ASC, d.id ASC").Scan(&out).Error
	return out, err
}

// TravarUsuario trava a linha do usuário (SELECT ... FOR UPDATE) dentro da transação. Serializa as
// inscrições do mesmo atleta em desafios diferentes (choque de horário): a segunda espera a primeira
// confirmar e, como a leitura consistente dela começa depois da trava, enxerga a inscrição nova.
func (r *DesafioRepository) TravarUsuario(idUsuario uint) error {
	var ids []uint
	return r.db.Raw("SELECT id FROM usuarios WHERE id = ? FOR UPDATE", idUsuario).Scan(&ids).Error
}
