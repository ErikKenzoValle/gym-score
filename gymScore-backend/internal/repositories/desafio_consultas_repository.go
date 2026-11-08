package repositories

import (
	"time"

	"gymScore-backend/internal/models"
)

// Consultas de leitura do atleta sobre desafios (ingressos, histórico), em arquivo próprio para não
// misturar com o CRUD de desafio_repository.go.

// IngressoLinha é uma inscrição do usuário com os dados do desafio e da academia.
type IngressoLinha struct {
	IDDesafio            uint
	Titulo               string
	Categoria            string
	Local                *string
	DataEvento           *time.Time
	Status               models.StatusDesafio
	ValorEntradaCentavos int64
	IDAcademia           *uint
	NomeFantasia         *string
	CodigoIngresso       string
	Presente             bool
	PresenteEm           *time.Time
}

// ListarIngressos devolve as inscrições do usuário (qualquer status do desafio), as de evento mais
// recente primeiro.
func (r *DesafioRepository) ListarIngressos(idUsuario uint) ([]IngressoLinha, error) {
	var linhas []IngressoLinha
	err := r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Joins("LEFT JOIN academias a ON a.id = d.id_academia").
		Select(`d.id AS id_desafio, d.titulo, d.categoria, d.local, d.data_evento, d.status,
			d.valor_entrada_centavos, d.id_academia, a.nome_fantasia, p.codigo_ingresso, p.presente, p.presente_em`).
		Where("p.id_usuario = ?", idUsuario).
		Order("COALESCE(d.data_evento, d.criado_em) DESC, d.id DESC").
		Scan(&linhas).Error
	return linhas, err
}

// HistoricoLinha é um desafio encerrado em que o usuário esteve inscrito, com a posição no pódio.
type HistoricoLinha struct {
	IDDesafio            uint
	Titulo               string
	Categoria            string
	Local                *string
	DataEvento           *time.Time
	DataEncerramento     *time.Time
	Status               models.StatusDesafio
	ValorEntradaCentavos int64
	IDAcademia           *uint
	NomeFantasia         *string
	Participantes        int
	Presente             bool
	Vencedor             bool
	PremioCentavos       int64
	Posicao              *int
	Marca                *string
	CriadoEm             time.Time
}

// historicoMax limita o histórico devolvido (o perfil calcula as estatísticas sobre a lista inteira).
const historicoMax = 500

// ListarHistorico devolve os desafios ENCERRADOS em que o usuário esteve inscrito, do mais recente
// ao mais antigo, com a posição, a marca e o prêmio dele (nunca os dos demais participantes).
func (r *DesafioRepository) ListarHistorico(idUsuario uint) ([]HistoricoLinha, error) {
	var linhas []HistoricoLinha
	err := r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Joins("LEFT JOIN academias a ON a.id = d.id_academia").
		Joins("LEFT JOIN podio_desafios pd ON pd.id_desafio = p.id_desafio AND pd.id_participante = p.id_usuario").
		Select(`d.id AS id_desafio, d.titulo, d.categoria, d.local, d.data_evento, d.data_encerramento, d.status,
			d.valor_entrada_centavos, d.id_academia, a.nome_fantasia, d.criado_em,
			(SELECT COUNT(*) FROM desafio_participantes x WHERE x.id_desafio = d.id) AS participantes,
			p.presente, p.vencedor, p.premio_centavos, pd.posicao, pd.marca`).
		Where("p.id_usuario = ? AND d.status = ?", idUsuario, models.StatusEncerrado).
		Order("COALESCE(d.data_evento, d.data_encerramento, d.criado_em) DESC, d.id DESC").
		Limit(historicoMax).
		Scan(&linhas).Error
	return linhas, err
}

// ContarInscricoesAtivas conta as inscrições do usuário em desafios abertos ou em andamento.
func (r *DesafioRepository) ContarInscricoesAtivas(idUsuario uint) (int64, error) {
	var total int64
	err := r.db.Table("desafio_participantes p").
		Joins("JOIN desafios d ON d.id = p.id_desafio").
		Where("p.id_usuario = ? AND d.status IN ?", idUsuario,
			[]models.StatusDesafio{models.StatusAberto, models.StatusEmAndamento}).
		Count(&total).Error
	return total, err
}
