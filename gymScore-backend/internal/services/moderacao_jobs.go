package services

import (
	"fmt"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Jobs da moderação (agendador de internal/jobs). Idempotentes e seguros com N instâncias: o
// escalonamento usa FOR UPDATE SKIP LOCKED + CAS; o fim de suspensão trava a linha do usuário e só age
// se suspenso_ate ainda estiver vencido.

// EscalonarDenuncias sobe ao admin as denúncias da academia com prazo vencido (ator sistema).
func (s *moderacaoService) EscalonarDenuncias(limite int) (int, error) {
	agora := s.now()
	type aviso struct {
		idAcademia uint
		idDenuncia uint
	}
	var avisos []aviso
	err := s.db.Transaction(func(tx *gorm.DB) error {
		ids, err := s.denuncias.EscalonaveisVencidas(tx, agora, limite)
		if err != nil {
			return err
		}
		for _, id := range ids {
			d, err := s.denuncias.Buscar(tx, id, false)
			if err != nil {
				return err
			}
			if d == nil {
				continue
			}
			ok, err := s.denuncias.Encaminhar(tx, id, nil, agora)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := s.registrar(tx, atorSistema, EventoDenunciaEscalonada, models.EntidadeDenuncia, id, d.IDDesafio,
				"Prazo da academia vencido", map[string]any{"jurisdicao": d.Jurisdicao, "status": d.Status, "prazo_academia_ate": d.PrazoAcademiaAte},
				map[string]any{"jurisdicao": models.JurisdicaoAdmin, "status": models.DenunciaAberta}); err != nil {
				return err
			}
			if d.IDAcademiaJulgadora != nil {
				avisos = append(avisos, aviso{idAcademia: *d.IDAcademiaJulgadora, idDenuncia: id})
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, a := range avisos {
		if dono, err := s.consultas.UsuarioDaAcademia(a.idAcademia); err == nil && dono != nil {
			s.notif.Notificar(*dono, models.NotifDenunciaEscalonada, "Denúncia encaminhada ao admin",
				fmt.Sprintf("O prazo para julgar a denúncia #%d terminou e ela foi encaminhada à equipe GymScore.", a.idDenuncia),
				"/academia/denuncias")
		}
	}
	return len(avisos), nil
}

// EncerrarSuspensoes limpa suspenso_ate vencido (recalculando a partir das punições ainda vigentes),
// registra punicao_expirada e avisa o usuário. O acesso NÃO depende deste job: middleware e contexto()
// comparam suspenso_ate com o relógio em tempo real.
func (s *moderacaoService) EncerrarSuspensoes(limite int) (int, error) {
	agora := s.now()
	ids, err := s.punicoes.UsuariosComSuspensaoVencida(agora, limite)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		var avisar bool
		err := s.db.Transaction(func(tx *gorm.DB) error {
			anterior, existe, err := s.consultas.TravarUsuario(tx, id)
			if err != nil || !existe || anterior == nil || anterior.After(agora) {
				return err // outra instância já tratou (ou a suspensão foi estendida)
			}
			estado, _, err := s.punicoes.RecalcularEstadoConta(tx, id, agora)
			if err != nil {
				return err
			}
			// suspenso_ate é arredondado para cima em até 1 s: as suspensões que o definiam têm fim em
			// (anterior - 1 s, anterior].
			vencidas, err := s.consultas.SuspensoesEncerradas(tx, id, anterior.Add(-time.Second), *anterior)
			if err != nil {
				return err
			}
			for _, p := range vencidas {
				if err := s.registrar(tx, atorSistema, EventoPunicaoExpirada, models.EntidadePunicao, p.ID, p.IDDesafio, "",
					map[string]any{"fim": p.Fim}, map[string]any{"vigente": false, "suspenso_ate": estado.SuspensoAte}); err != nil {
					return err
				}
			}
			avisar = estado.SuspensoAte == nil && estado.BanidoEm == nil
			n++
			return nil
		})
		if err != nil {
			return n, err
		}
		if avisar {
			s.notif.Notificar(id, models.NotifSuspensaoEncerrada, "Suspensão encerrada",
				"Sua suspensão terminou. Você já pode voltar a participar dos desafios.", "/menu")
		}
	}
	return n, nil
}
