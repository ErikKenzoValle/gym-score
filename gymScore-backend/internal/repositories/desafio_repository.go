package repositories

import (
	"errors"
	"strings"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Ordenações da listagem de desafios.
const (
	OrdemRecentes   = ""            // criado_em DESC (padrão)
	OrdemEventoProx = "evento_prox" // data_evento ASC (feed: o mais próximo primeiro)
	OrdemEventoRec  = "evento_rec"  // data_evento DESC (histórico)
)

// FiltroDesafios parametriza a listagem (paginação e filtro feitos no SQL).
type FiltroDesafios struct {
	IDUsuario uint
	Status    models.StatusDesafio // vazio = sem filtro
	// Statuses filtra por vários status de uma vez (combina com Status: ambos valem). Vazio = sem filtro.
	Statuses    []models.StatusDesafio
	SomenteMeus bool // true: apenas desafios em que o usuário participa/administra
	// Todos ignora o filtro de visibilidade (uso administrativo: dashboard de academia, agenda de instrutor).
	Todos       bool
	Categoria   string // vazio = qualquer
	Busca       string // texto livre em título, local e nome fantasia da academia
	IDAcademia  uint   // 0 = qualquer
	IDInstrutor uint   // 0 = qualquer (agenda do instrutor)
	// VagaAberta mantém só desafios com a vaga de instrutor aberta e sem escolhido (aba "Vagas").
	VagaAberta bool
	// Futuros mantém só desafios com data_evento depois de Agora (legados sem data ficam de fora).
	Futuros bool
	Agora   time.Time
	Ordem   string
	Pagina  int
	Limite  int
}

// DesafioRepository acesso a dados de desafios, participantes e pódio.
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

// BuscarDetalhe retorna o desafio com academia, instrutor e participantes (ordem de entrada).
func (r *DesafioRepository) BuscarDetalhe(id uint) (*models.Desafio, error) {
	var d models.Desafio
	err := r.db.Preload("Academia").Preload("Instrutor.Usuario").
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

// Atualizar grava os campos informados (nomes de coluna). Chamar com o desafio travado.
func (r *DesafioRepository) Atualizar(id uint, campos map[string]any) error {
	if len(campos) == 0 {
		return nil
	}
	return r.db.Model(&models.Desafio{}).Where("id = ?", id).Updates(campos).Error
}

// participaSQL: o usuário (?) está inscrito no desafio.
const participaSQL = `EXISTS (SELECT 1 FROM desafio_participantes p WHERE p.id_desafio = desafios.id AND p.id_usuario = ?)`

// envolvidoSQL: o usuário (?) participa, é a academia dona ou o instrutor designado.
const envolvidoSQL = `(desafios.id_criador = ?
	OR desafios.id_academia IN (SELECT a.id FROM academias a WHERE a.id_usuario = ?)
	OR desafios.id_instrutor IN (SELECT i.id FROM instrutores i WHERE i.id_usuario = ?)
	OR ` + participaSQL + `)`

// Listar retorna uma página de desafios e o total (filtros e paginação no SQL). Cada desafio vem
// com academia, instrutor, contagens (QtdParticipantes/QtdPresentes) e, se IDUsuario != 0, a
// inscrição dele (MinhaParticipacao). Os participantes NÃO são carregados (use BuscarDetalhe).
func (r *DesafioRepository) Listar(f FiltroDesafios) ([]models.Desafio, int64, error) {
	q := r.db.Model(&models.Desafio{})
	switch {
	case f.Todos:
		// Sem filtro de visibilidade: uso administrativo (dashboard de academia, agenda de instrutor).
	case f.SomenteMeus:
		q = q.Where(envolvidoSQL, f.IDUsuario, f.IDUsuario, f.IDUsuario, f.IDUsuario)
	default:
		// Abertos são públicos; os demais status só para quem está envolvido.
		q = q.Where("(desafios.status = 'aberto' OR "+envolvidoSQL+")", f.IDUsuario, f.IDUsuario, f.IDUsuario, f.IDUsuario)
	}
	if f.Status != "" {
		q = q.Where("desafios.status = ?", f.Status)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("desafios.status IN ?", f.Statuses)
	}
	if f.Categoria != "" {
		q = q.Where("desafios.categoria = ?", f.Categoria)
	}
	if f.IDAcademia != 0 {
		q = q.Where("desafios.id_academia = ?", f.IDAcademia)
	}
	if f.VagaAberta {
		q = q.Where("desafios.instrutor_selecao = ? AND desafios.id_instrutor IS NULL", models.SelecaoVagaAberta)
	}
	if f.IDInstrutor != 0 {
		q = q.Where("desafios.id_instrutor = ?", f.IDInstrutor)
	}
	if f.Futuros {
		q = q.Where("desafios.data_evento > ?", f.Agora)
	}
	if busca := strings.TrimSpace(f.Busca); busca != "" {
		like := "%" + escaparLike(busca) + "%"
		q = q.Where(`(desafios.titulo LIKE ? OR desafios.local LIKE ? OR desafios.id_academia IN
			(SELECT a.id FROM academias a WHERE a.nome_fantasia LIKE ?))`, like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	switch f.Ordem {
	case OrdemEventoProx:
		q = q.Order("desafios.data_evento ASC, desafios.id ASC")
	case OrdemEventoRec:
		q = q.Order("COALESCE(desafios.data_evento, desafios.criado_em) DESC, desafios.id DESC")
	default:
		q = q.Order("desafios.criado_em DESC, desafios.id DESC")
	}

	var desafios []models.Desafio
	err := q.Preload("Academia").Preload("Instrutor.Usuario").
		Offset((f.Pagina - 1) * f.Limite).Limit(f.Limite).
		Find(&desafios).Error
	if err != nil || len(desafios) == 0 {
		return desafios, total, err
	}
	return desafios, total, r.enriquecer(desafios, f.IDUsuario)
}

// escaparLike neutraliza curingas do LIKE vindos do usuário.
func escaparLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// enriquecer preenche, em lote (2 queries), as contagens e a inscrição do usuário.
func (r *DesafioRepository) enriquecer(desafios []models.Desafio, idUsuario uint) error {
	ids := make([]uint, len(desafios))
	for i := range desafios {
		ids[i] = desafios[i].ID
	}
	var linhas []struct {
		IDDesafio     uint
		Participantes int
		Presentes     int
	}
	if err := r.db.Model(&models.DesafioParticipante{}).
		Select("id_desafio, COUNT(*) AS participantes, COALESCE(SUM(presente), 0) AS presentes").
		Where("id_desafio IN ?", ids).Group("id_desafio").Scan(&linhas).Error; err != nil {
		return err
	}
	cont := make(map[uint][2]int, len(linhas))
	for _, l := range linhas {
		cont[l.IDDesafio] = [2]int{l.Participantes, l.Presentes}
	}
	var minhas map[uint]models.DesafioParticipante
	if idUsuario != 0 {
		var err error
		if minhas, err = r.ParticipacoesDoUsuario(idUsuario, ids); err != nil {
			return err
		}
	}
	for i := range desafios {
		c := cont[desafios[i].ID]
		desafios[i].QtdParticipantes, desafios[i].QtdPresentes = c[0], c[1]
		if p, ok := minhas[desafios[i].ID]; ok {
			desafios[i].MinhaParticipacao = &p
		}
	}
	return nil
}

// ParticipacoesDoUsuario devolve, por id de desafio, a inscrição do usuário (apenas os desafios em ids).
func (r *DesafioRepository) ParticipacoesDoUsuario(idUsuario uint, ids []uint) (map[uint]models.DesafioParticipante, error) {
	var ps []models.DesafioParticipante
	if err := r.db.Where("id_usuario = ? AND id_desafio IN ?", idUsuario, ids).Find(&ps).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]models.DesafioParticipante, len(ps))
	for _, p := range ps {
		out[p.IDDesafio] = p
	}
	return out, nil
}

// Avaliados devolve os ids (entre ids) que o usuário já avaliou.
func (r *DesafioRepository) Avaliados(idUsuario uint, ids []uint) (map[uint]bool, error) {
	var avaliados []uint
	if err := r.db.Model(&models.AvaliacaoDesafio{}).
		Where("id_usuario = ? AND id_desafio IN ?", idUsuario, ids).Pluck("id_desafio", &avaliados).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]bool, len(avaliados))
	for _, id := range avaliados {
		out[id] = true
	}
	return out, nil
}

// ─── Participantes ───────────────────────────────────────────────────────────

// AdicionarParticipante inscreve um usuário em um desafio. CodigoIngresso deve vir preenchido.
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

// ContarPresentes retorna quantos inscritos têm a presença confirmada.
func (r *DesafioRepository) ContarPresentes(idDesafio uint) (int64, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND presente = ?", idDesafio, true).Count(&total).Error
	return total, err
}

// UsuarioParticipa indica se o usuário já está inscrito no desafio
// UsuarioBloqueadoNaAcademia diz se há punição bloqueio_academia vigente do usuário na academia
// (mesma regra de vigência de PunicaoRepository: não revogada, iniciada e sem fim vencido).
func (r *DesafioRepository) UsuarioBloqueadoNaAcademia(idUsuario, idAcademia uint, agora time.Time) (bool, error) {
	var n int64
	err := r.db.Model(&models.Punicao{}).
		Where("id_academia = ? AND id_usuario = ? AND tipo = ? AND "+condVigente, idAcademia, idUsuario, models.PunicaoBloqueioAcademia, agora, agora).
		Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *DesafioRepository) UsuarioParticipa(idDesafio, idUsuario uint) (bool, error) {
	var total int64
	err := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Count(&total).Error
	return total > 0, err
}

// ListarParticipantes retorna os participantes ordenados por id_usuario (ordem estável de locks/créditos).
func (r *DesafioRepository) ListarParticipantes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var ps []models.DesafioParticipante
	err := r.db.Where("id_desafio = ?", idDesafio).Order("id_usuario ASC").Find(&ps).Error
	return ps, err
}

// BuscarParticipante devolve a inscrição (com o usuário) ou nil.
func (r *DesafioRepository) BuscarParticipante(idDesafio, idUsuario uint) (*models.DesafioParticipante, error) {
	var p models.DesafioParticipante
	err := r.db.Preload("Usuario").Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// BuscarParticipantePorCodigo devolve a inscrição cujo ingresso tem esse código NESTE desafio
// (código de outro desafio não casa) ou nil.
func (r *DesafioRepository) BuscarParticipantePorCodigo(idDesafio uint, codigo string) (*models.DesafioParticipante, error) {
	var p models.DesafioParticipante
	err := r.db.Preload("Usuario").Where("id_desafio = ? AND codigo_ingresso = ?", idDesafio, codigo).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// MarcarPresente confirma a presença (presente=1 e presente_em) num UPDATE condicional.
// Retorna true se marcou agora, false se o participante já estava presente.
func (r *DesafioRepository) MarcarPresente(idDesafio, idUsuario uint, em time.Time) (bool, error) {
	res := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ? AND presente_em IS NULL", idDesafio, idUsuario).
		Updates(map[string]any{"presente": true, "presente_em": em})
	return res.RowsAffected > 0, res.Error
}

// MarcarPresenteComOrigem é o MarcarPresente que também grava quem (staff) e como (QR/manual)
// confirmou a presença. Retorna true se marcou agora, false se o participante já estava presente.
func (r *DesafioRepository) MarcarPresenteComOrigem(idDesafio, idUsuario, marcadoPor uint, modo models.ModoPresenca, em time.Time) (bool, error) {
	res := r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ? AND presente_em IS NULL", idDesafio, idUsuario).
		Updates(map[string]any{"presente": true, "presente_em": em,
			"presenca_marcada_por": marcadoPor, "presenca_modo": modo})
	return res.RowsAffected > 0, res.Error
}

// ListarPresentes retorna os participantes presentes (com usuário), por ordem de chegada.
func (r *DesafioRepository) ListarPresentes(idDesafio uint) ([]models.DesafioParticipante, error) {
	var ps []models.DesafioParticipante
	err := r.db.Preload("Usuario").Where("id_desafio = ? AND presente = ?", idDesafio, true).
		Order("presente_em ASC, id ASC").Find(&ps).Error
	return ps, err
}

// RegistrarPremio grava o prêmio do participante e, se for o 1º colocado, a marca de vencedor.
func (r *DesafioRepository) RegistrarPremio(idDesafio, idUsuario uint, premio int64, vencedor bool) error {
	return r.db.Model(&models.DesafioParticipante{}).
		Where("id_desafio = ? AND id_usuario = ?", idDesafio, idUsuario).
		Updates(map[string]any{"premio_centavos": premio, "vencedor": vencedor}).Error
}

// ─── Pódio ───────────────────────────────────────────────────────────────────

// InserirPodio grava uma posição do pódio.
func (r *DesafioRepository) InserirPodio(p *models.PodioDesafio) error {
	return r.db.Create(p).Error
}

// ListarPodio devolve o pódio do desafio por posição, com o usuário.
func (r *DesafioRepository) ListarPodio(idDesafio uint) ([]models.PodioDesafio, error) {
	var lista []models.PodioDesafio
	err := r.db.Preload("Participante").Where("id_desafio = ?", idDesafio).Order("posicao ASC").Find(&lista).Error
	return lista, err
}

// ─── Consultas de outros fluxos ──────────────────────────────────────────────

// ListarHistoricoUsuario retorna os desafios encerrados em que idUsuario participou. A Participantes
// vem filtrada só para a linha de idUsuario — é o resultado (vencedor/prêmio) dele que importa aqui,
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

// IDsVencidos devolve ids de desafios `aberto`/`em_andamento` com data_evento anterior a `limite`
// (já descontada a carência), mais antigos primeiro, no máximo `max`. Usa idx_desafios_feed
// (status, data_evento).
func (r *DesafioRepository) IDsVencidos(limite time.Time, max int) ([]uint, error) {
	var ids []uint
	// Desafio em_andamento com o resultado anulado aguardando relançamento (Q3): a carência conta de
	// resultado_anulado_em, não da data do evento.
	err := r.db.Model(&models.Desafio{}).
		Where("(status = ? AND data_evento IS NOT NULL AND data_evento < ?) OR (status = ? AND "+sqlVencidoAndamento+")",
			models.StatusAberto, limite, models.StatusEmAndamento, limite, limite).
		Order("id ASC").Limit(max).Pluck("id", &ids).Error
	return ids, err
}

// PerfisParticipantes devolve, por id de usuário, o perfil atual de cada inscrito no desafio.
func (r *DesafioRepository) PerfisParticipantes(idDesafio uint) (map[uint]string, error) {
	var linhas []struct {
		ID     uint
		Perfil string
	}
	err := r.db.Table("desafio_participantes dp").Select("u.id, u.perfil").
		Joins("JOIN usuarios u ON u.id = dp.id_usuario").
		Where("dp.id_desafio = ?", idDesafio).Scan(&linhas).Error
	out := make(map[uint]string, len(linhas))
	for _, l := range linhas {
		out[l.ID] = l.Perfil
	}
	return out, err
}
