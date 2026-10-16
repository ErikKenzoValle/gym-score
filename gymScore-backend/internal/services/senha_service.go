package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"golang.org/x/crypto/bcrypt"
)

// ValidadeTokenReset é o tempo de vida do link de recuperação de senha.
const ValidadeTokenReset = 30 * time.Minute

// SenhaService trata a recuperação de senha por e-mail (token aleatório de uso único).
type SenhaService interface {
	// EsqueciSenha nunca revela se o e-mail existe: erros internos só vão para o log.
	EsqueciSenha(email string)
	RedefinirSenha(req *models.RedefinirSenhaRequest) error
}

type senhaService struct {
	usuarios   repositories.UsuarioRepository
	tokens     repositories.ResetSenhaRepository
	mailer     Mailer
	baseURL    string
	agora      func() time.Time
	assincrono bool
}

// NovoSenhaService cria o serviço. O envio do e-mail roda em goroutine para o tempo de
// resposta não distinguir e-mails cadastrados dos não cadastrados.
func NovoSenhaService(
	usuarios repositories.UsuarioRepository,
	tokens repositories.ResetSenhaRepository,
	mailer Mailer,
	baseURL string,
) SenhaService {
	return &senhaService{usuarios: usuarios, tokens: tokens, mailer: mailer,
		baseURL: strings.TrimRight(baseURL, "/"), agora: time.Now, assincrono: true}
}

// NovoSenhaServiceSincrono é usado nos testes (envio e relógio determinísticos).
func NovoSenhaServiceSincrono(
	usuarios repositories.UsuarioRepository,
	tokens repositories.ResetSenhaRepository,
	mailer Mailer,
	baseURL string,
	agora func() time.Time,
) SenhaService {
	return &senhaService{usuarios: usuarios, tokens: tokens, mailer: mailer,
		baseURL: strings.TrimRight(baseURL, "/"), agora: agora}
}

// HashToken devolve o SHA-256 em hexadecimal (64 chars) — é o que vai para o banco.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *senhaService) EsqueciSenha(email string) {
	usuario, err := s.usuarios.BuscarPorEmail(strings.TrimSpace(email))
	if err != nil {
		log.Printf("[SENHA] erro ao buscar usuário para reset: %v", err)
		return
	}
	if usuario == nil {
		return
	}

	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		log.Printf("[SENHA] erro ao gerar token: %v", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(bruto)

	if err := s.tokens.Criar(&models.PasswordResetToken{
		IDUsuario: usuario.ID,
		TokenHash: HashToken(token),
		ExpiraEm:  s.agora().Add(ValidadeTokenReset),
	}); err != nil {
		log.Printf("[SENHA] erro ao salvar token: %v", err)
		return
	}

	link := fmt.Sprintf("%s/redefinir-senha?token=%s", s.baseURL, url.QueryEscape(token))
	corpo := fmt.Sprintf("Olá, %s!\n\nPara redefinir sua senha acesse o link abaixo (válido por 30 minutos, uso único):\n%s\n\nSe você não pediu isso, ignore este e-mail.\n", usuario.Nome, link)
	envio := func() {
		if err := s.mailer.Enviar(usuario.Email, "GymScore - redefinição de senha", corpo); err != nil {
			log.Printf("[SENHA] falha ao enviar e-mail de reset: %v", err)
		}
	}
	if s.assincrono {
		go envio()
	} else {
		envio()
	}
}

func (s *senhaService) RedefinirSenha(req *models.RedefinirSenhaRequest) error {
	if len(req.NovaSenha) < 8 {
		return erroRequisicao("nova senha deve ter pelo menos 8 caracteres")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("erro ao processar nova senha: %w", err)
	}
	ok, err := s.tokens.Consumir(HashToken(req.Token), s.agora(), string(hash))
	if err != nil {
		return err
	}
	if !ok {
		return erroRequisicao("Link inválido, expirado ou já utilizado")
	}
	return nil
}
