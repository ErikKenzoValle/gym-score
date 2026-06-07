package services

import (
	"crypto/rand"
	"errors"
	"fmt"

	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
)

// TreinoService define as operações de negócio para treinos.
type TreinoService interface {
	Criar(req *models.CriarTreinoRequest) (*models.Treino, error)
	ListarPorUsuario(idUsuario uint) ([]models.Treino, error)
	BuscarPorID(id uint) (*models.Treino, error)
	Deletar(id, idUsuario uint) error
	Importar(req *models.ImportarTreinoRequest) (*models.Treino, error)
	ConcluirExercicio(req *models.ConcluirExercicioRequest) error
	Radar(idUsuario uint) ([]models.RadarGrupo, error)
}

type treinoService struct {
	treinoRepo  repositories.TreinoRepository
	usuarioRepo repositories.UsuarioRepository
}

func NovoTreinoService(
	treinoRepo repositories.TreinoRepository,
	usuarioRepo repositories.UsuarioRepository,
) TreinoService {
	return &treinoService{treinoRepo: treinoRepo, usuarioRepo: usuarioRepo}
}

const codigoAlfabeto = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // sem caracteres ambíguos (0/O, 1/I)

// gerarCodigo cria um código aleatório de 6 caracteres.
func gerarCodigo() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	out := make([]byte, 6)
	for i, v := range b {
		out[i] = codigoAlfabeto[int(v)%len(codigoAlfabeto)]
	}
	return string(out)
}

// gerarCodigoUnico tenta gerar um código que ainda não existe no banco.
func (s *treinoService) gerarCodigoUnico() (string, error) {
	for tentativa := 0; tentativa < 8; tentativa++ {
		c := gerarCodigo()
		existe, err := s.treinoRepo.CodigoExiste(c)
		if err != nil {
			return "", err
		}
		if !existe {
			return c, nil
		}
	}
	return "", errors.New("não foi possível gerar um código único")
}

func (s *treinoService) Criar(req *models.CriarTreinoRequest) (*models.Treino, error) {
	if req.IDUsuario == 0 {
		return nil, errors.New("usuário não identificado")
	}
	if req.Nome == "" {
		return nil, errors.New("nome do treino é obrigatório")
	}
	if len(req.Exercicios) == 0 {
		return nil, errors.New("adicione ao menos um exercício")
	}

	codigo, err := s.gerarCodigoUnico()
	if err != nil {
		return nil, err
	}

	treino := &models.Treino{
		IDUsuario: req.IDUsuario,
		Nome:      req.Nome,
		Descricao: req.Descricao,
		Codigo:    codigo,
	}
	for i, ex := range req.Exercicios {
		treino.Exercicios = append(treino.Exercicios, models.TreinoExercicio{
			Nome:          ex.Nome,
			GrupoMuscular: ex.GrupoMuscular,
			Ordem:         i,
			Series:        ex.Series,
			Repeticoes:    ex.Repeticoes,
		})
	}

	if err := s.treinoRepo.Criar(treino); err != nil {
		return nil, fmt.Errorf("erro ao criar treino: %w", err)
	}
	return treino, nil
}

func (s *treinoService) ListarPorUsuario(idUsuario uint) ([]models.Treino, error) {
	return s.treinoRepo.ListarPorUsuario(idUsuario)
}

func (s *treinoService) BuscarPorID(id uint) (*models.Treino, error) {
	return s.treinoRepo.BuscarPorID(id)
}

func (s *treinoService) Deletar(id, idUsuario uint) error {
	treino, err := s.treinoRepo.BuscarPorID(id)
	if err != nil {
		return err
	}
	if treino == nil {
		return errors.New("treino não encontrado")
	}
	if treino.IDUsuario != idUsuario {
		return errors.New("você só pode excluir os seus próprios treinos")
	}
	return s.treinoRepo.Deletar(id)
}

// Importar clona um treino existente (pelo código) para o usuário atual.
func (s *treinoService) Importar(req *models.ImportarTreinoRequest) (*models.Treino, error) {
	if req.IDUsuario == 0 {
		return nil, errors.New("usuário não identificado")
	}
	original, err := s.treinoRepo.BuscarPorCodigo(req.Codigo)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, errors.New("nenhum treino encontrado com esse código")
	}
	if original.IDUsuario == req.IDUsuario {
		return nil, errors.New("este treino já é seu")
	}

	codigo, err := s.gerarCodigoUnico()
	if err != nil {
		return nil, err
	}

	clone := &models.Treino{
		IDUsuario: req.IDUsuario,
		Nome:      original.Nome,
		Descricao: original.Descricao,
		Codigo:    codigo,
	}
	for _, ex := range original.Exercicios {
		clone.Exercicios = append(clone.Exercicios, models.TreinoExercicio{
			Nome:          ex.Nome,
			GrupoMuscular: ex.GrupoMuscular,
			Ordem:         ex.Ordem,
			Series:        ex.Series,
			Repeticoes:    ex.Repeticoes,
		})
	}
	if err := s.treinoRepo.Criar(clone); err != nil {
		return nil, fmt.Errorf("erro ao importar treino: %w", err)
	}
	return clone, nil
}

func (s *treinoService) ConcluirExercicio(req *models.ConcluirExercicioRequest) error {
	if req.IDUsuario == 0 {
		return errors.New("usuário não identificado")
	}
	if req.GrupoMuscular == "" {
		return errors.New("grupo muscular é obrigatório")
	}
	return s.treinoRepo.RegistrarConclusao(&models.TreinoConclusao{
		IDUsuario:     req.IDUsuario,
		IDTreino:      req.IDTreino,
		GrupoMuscular: req.GrupoMuscular,
	})
}

func (s *treinoService) Radar(idUsuario uint) ([]models.RadarGrupo, error) {
	return s.treinoRepo.RadarPorUsuario(idUsuario)
}
