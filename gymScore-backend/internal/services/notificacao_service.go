package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"
)

// Notificador envia notificações in-app. É best-effort: falhas só vão para o log e
// nunca derrubam a operação de negócio que gerou o evento.
type Notificador interface {
	Notificar(idUsuario uint, tipo, titulo, mensagem, link string)
	NotificarVarios(ids []uint, tipo, titulo, mensagem, link string)
}

// NotificadorNulo descarta as notificações (testes unitários / dependência opcional).
type NotificadorNulo struct{}

func (NotificadorNulo) Notificar(uint, string, string, string, string)         {}
func (NotificadorNulo) NotificarVarios([]uint, string, string, string, string) {}

// NotificacaoService expõe o Notificador e as consultas do próprio usuário.
type NotificacaoService interface {
	Notificador
	Listar(idUsuario uint, pagina, limite int) (*models.ListaNotificacoesResponse, error)
	NaoLidas(idUsuario uint) (int64, error)
	MarcarLida(idUsuario, id uint) error
	MarcarTodasLidas(idUsuario uint) (int64, error)
}

type notificacaoService struct {
	repo  repositories.NotificacaoRepository
	agora func() time.Time
}

// NovoNotificacaoService cria o serviço de notificações.
func NovoNotificacaoService(repo repositories.NotificacaoRepository) NotificacaoService {
	return &notificacaoService{repo: repo, agora: time.Now}
}

func truncar(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return string(r)
}

func (s *notificacaoService) montar(id uint, tipo, titulo, msg, link string) models.Notificacao {
	n := models.Notificacao{IDUsuario: id, Tipo: tipo, Titulo: truncar(titulo, 150), Mensagem: truncar(msg, 500)}
	if link != "" {
		l := truncar(link, 255)
		n.Link = &l
	}
	return n
}

func (s *notificacaoService) Notificar(idUsuario uint, tipo, titulo, mensagem, link string) {
	n := s.montar(idUsuario, tipo, titulo, mensagem, link)
	if err := s.repo.Criar(&n); err != nil {
		log.Printf("[NOTIF] falha ao notificar usuário %d (%s): %v", idUsuario, tipo, err)
	}
}

func (s *notificacaoService) NotificarVarios(ids []uint, tipo, titulo, mensagem, link string) {
	ns := make([]models.Notificacao, 0, len(ids))
	for _, id := range ids {
		ns = append(ns, s.montar(id, tipo, titulo, mensagem, link))
	}
	if err := s.repo.CriarVarias(ns); err != nil {
		log.Printf("[NOTIF] falha ao notificar %d usuários (%s): %v", len(ids), tipo, err)
	}
}

func (s *notificacaoService) Listar(idUsuario uint, pagina, limite int) (*models.ListaNotificacoesResponse, error) {
	pagina, limite = normalizarPagina(pagina, limite)
	itens, total, err := s.repo.Listar(idUsuario, pagina, limite)
	if err != nil {
		return nil, err
	}
	naoLidas, err := s.repo.ContarNaoLidas(idUsuario)
	if err != nil {
		return nil, err
	}
	resp := &models.ListaNotificacoesResponse{
		Itens: make([]models.NotificacaoResponse, 0, len(itens)), Total: total, NaoLidas: naoLidas,
		Pagina: pagina, Limite: limite,
	}
	for _, n := range itens {
		r := models.NotificacaoResponse{ID: n.ID, Tipo: n.Tipo, Titulo: n.Titulo, Mensagem: n.Mensagem,
			Lida: n.LidaEm != nil, LidaEm: n.LidaEm, CriadoEm: n.CriadoEm}
		if n.Link != nil {
			r.Link = *n.Link
		}
		resp.Itens = append(resp.Itens, r)
	}
	return resp, nil
}

func (s *notificacaoService) NaoLidas(idUsuario uint) (int64, error) {
	return s.repo.ContarNaoLidas(idUsuario)
}

// MarcarLida é idempotente; 404 se a notificação não existir ou for de outro usuário.
func (s *notificacaoService) MarcarLida(idUsuario, id uint) error {
	existe, err := s.repo.MarcarLida(id, idUsuario, s.agora())
	if err != nil {
		return err
	}
	if !existe {
		return erroNaoEncontrado("Notificação não encontrada")
	}
	return nil
}

func (s *notificacaoService) MarcarTodasLidas(idUsuario uint) (int64, error) {
	return s.repo.MarcarTodasLidas(idUsuario, s.agora())
}

// reaisBR formata centavos como "R$ 12,34" para mensagens.
func reaisBR(centavos int64) string {
	return "R$ " + strings.Replace(formatarReais(utils.CentavosParaReais(centavos)), ".", ",", 1)
}

func formatarReais(v float64) string { return fmt.Sprintf("%.2f", v) }
