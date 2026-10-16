package services

import (
	"errors"
	"strings"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
)

// AmizadeService define as operações de negócio para amizades. O usuário que age
// é sempre o do JWT (userID); o alvo (idAmigo) vem do corpo.
type AmizadeService interface {
	AdicionarAmigo(userID, idAmigo uint) error
	AceitarAmizade(userID, idRemetente uint) error
	RecusarAmizade(userID, idRemetente uint) error
	RemoverAmigo(userID, idAmigo uint) error
	ListarAmigos(userID uint) ([]models.AmigoResponse, error)
}

type amizadeService struct {
	amizadeRepo repositories.AmizadeRepository
	usuarioRepo repositories.UsuarioRepository
	notif       Notificador
}

// NovoAmizadeService cria uma nova instância do serviço de amizades
func NovoAmizadeService(
	amizadeRepo repositories.AmizadeRepository,
	usuarioRepo repositories.UsuarioRepository,
	notif Notificador,
) AmizadeService {
	return &amizadeService{amizadeRepo: amizadeRepo, usuarioRepo: usuarioRepo, notif: notif}
}

// nomeDe devolve "Nome Sobrenome" do usuário para as mensagens (vazio se não achar).
func (s *amizadeService) nomeDe(id uint) string {
	if u, err := s.usuarioRepo.BuscarPorID(id); err == nil && u != nil {
		return strings.TrimSpace(u.Nome + " " + u.Sobrenome)
	}
	return "Alguém"
}

// AdicionarAmigo envia uma solicitação de amizade
func (s *amizadeService) AdicionarAmigo(userID, idAmigo uint) error {
	if userID == idAmigo {
		return erroRequisicao("não é possível adicionar a si mesmo como amigo")
	}
	amigo, err := s.usuarioRepo.BuscarPorID(idAmigo)
	if err != nil {
		return err
	}
	if amigo == nil {
		return erroNaoEncontrado("usuário não encontrado")
	}

	relacao, err := s.amizadeRepo.BuscarRelacao(userID, idAmigo)
	if err != nil {
		return err
	}
	if relacao != nil {
		switch {
		case relacao.Status == models.StatusAmizadeRecusada:
			// Permite reenviar uma solicitação que foi recusada
			ok, err := s.amizadeRepo.Reenviar(relacao.ID, userID, idAmigo)
			if err != nil {
				return err
			}
			if ok {
				s.notif.Notificar(idAmigo, models.NotifAmizadeRecebida, "Nova solicitação de amizade",
					s.nomeDe(userID)+" quer ser seu amigo.", "/amigos")
				return nil
			}
			return erroConflito("solicitação de amizade já existe ou vocês já são amigos")
		case relacao.Status == models.StatusAmizadePendente && relacao.IDUsuario == idAmigo:
			return erroConflito("esta pessoa já enviou uma solicitação para você; aceite-a")
		default:
			return erroConflito("solicitação de amizade já existe ou vocês já são amigos")
		}
	}

	err = s.amizadeRepo.Criar(&models.Amizade{
		IDUsuario: userID, IDAmigo: idAmigo, Status: models.StatusAmizadePendente,
	})
	if errors.Is(err, repositories.ErrAmizadeDuplicada) {
		return erroConflito("solicitação de amizade já existe ou vocês já são amigos")
	}
	if err == nil {
		s.notif.Notificar(idAmigo, models.NotifAmizadeRecebida, "Nova solicitação de amizade",
			s.nomeDe(userID)+" quer ser seu amigo.", "/amigos")
	}
	return err
}

// AceitarAmizade aceita uma solicitação pendente: SOMENTE o destinatário (userID) pode aceitar.
func (s *amizadeService) AceitarAmizade(userID, idRemetente uint) error {
	ok, err := s.amizadeRepo.Responder(idRemetente, userID, models.StatusAmizadeAceita)
	if err != nil {
		return err
	}
	if !ok {
		return erroNaoEncontrado("solicitação de amizade pendente não encontrada")
	}
	s.notif.Notificar(idRemetente, models.NotifAmizadeAceita, "Solicitação aceita",
		s.nomeDe(userID)+" aceitou sua solicitação de amizade.", "/amigos")
	return nil
}

// RecusarAmizade recusa uma solicitação pendente recebida.
func (s *amizadeService) RecusarAmizade(userID, idRemetente uint) error {
	ok, err := s.amizadeRepo.Responder(idRemetente, userID, models.StatusAmizadeRecusada)
	if err != nil {
		return err
	}
	if !ok {
		return erroNaoEncontrado("solicitação de amizade pendente não encontrada")
	}
	return nil
}

// RemoverAmigo desfaz a amizade (ou cancela a solicitação pendente) entre userID e idAmigo.
func (s *amizadeService) RemoverAmigo(userID, idAmigo uint) error {
	ok, err := s.amizadeRepo.Remover(userID, idAmigo)
	if err != nil {
		return err
	}
	if !ok {
		return erroNaoEncontrado("amizade não encontrada")
	}
	return nil
}

// ListarAmigos retorna amigos aceitos e solicitações pendentes do usuário (sem e-mail/CPF)
func (s *amizadeService) ListarAmigos(userID uint) ([]models.AmigoResponse, error) {
	amizades, err := s.amizadeRepo.ListarAmigos(userID)
	if err != nil {
		return nil, err
	}
	amigos := make([]models.AmigoResponse, 0, len(amizades))
	for _, a := range amizades {
		outro := a.Usuario // o "amigo" é o outro lado da relação
		if a.IDUsuario == userID {
			outro = a.Amigo
		}
		if outro == nil {
			continue
		}
		amigos = append(amigos, models.AmigoResponse{
			ID:        outro.ID,
			Nome:      outro.Nome,
			Sobrenome: outro.Sobrenome,
			Status:    string(a.Status),
			Recebida:  a.Status == models.StatusAmizadePendente && a.IDAmigo == userID,
		})
	}
	return amigos, nil
}
