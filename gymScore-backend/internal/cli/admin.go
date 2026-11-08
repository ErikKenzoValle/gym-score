// Package cli implementa os comandos de linha de comando do binário (hoje: `admin ...`).
//
//	go run . admin criar-superadmin --email admin@exemplo.com --nome "Fulano"
//	go run . admin resetar-senha --email admin@exemplo.com
//
// A senha NUNCA é aceita por flag ou argumento: é lida do terminal sem eco (duas vezes). Sem TTY
// (scripts, testes automatizados) ela é lida do stdin, uma linha por leitura.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
	"gorm.io/gorm"
)

// Uso é o texto de ajuda dos comandos admin.
const Uso = `Uso:
  admin criar-superadmin --email <email> --nome <nome>
  admin resetar-senha --email <email>
A senha é pedida no terminal (sem eco, duas vezes, mínimo 12 caracteres); nunca por argumento.`

// LeitorSenha lê uma senha mostrando o prompt (sem eco quando houver terminal).
type LeitorSenha func(prompt string) (string, error)

// comando é o resultado do parse dos argumentos.
type comando struct {
	nome      string // criar-superadmin | resetar-senha
	email     string
	nomeAdmin string
}

// parse valida os argumentos: só os flags conhecidos; qualquer argumento extra (inclusive tentativa de
// passar a senha) é recusado.
func parse(args []string) (*comando, error) {
	if len(args) == 0 {
		return nil, errors.New(Uso)
	}
	for _, a := range args {
		l := strings.ToLower(a)
		if strings.HasPrefix(l, "-") && (strings.Contains(l, "senha") || strings.Contains(l, "password") || l == "-p") {
			return nil, errors.New("a senha nunca é aceita por argumento: ela é pedida no terminal")
		}
	}
	cmd := &comando{nome: args[0]}
	fs := flag.NewFlagSet("admin "+args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cmd.email, "email", "", "e-mail do administrador")
	switch cmd.nome {
	case "criar-superadmin":
		fs.StringVar(&cmd.nomeAdmin, "nome", "", "nome do administrador")
	case "resetar-senha":
	default:
		return nil, fmt.Errorf("comando desconhecido %q\n%s", cmd.nome, Uso)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return nil, fmt.Errorf("%v\n%s", err, Uso)
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("argumentos inesperados: %v\n%s", fs.Args(), Uso)
	}
	cmd.email = strings.ToLower(strings.TrimSpace(cmd.email))
	cmd.nomeAdmin = strings.TrimSpace(cmd.nomeAdmin)
	if !utils.ValidarEmail(cmd.email) {
		return nil, errors.New("--email ausente ou inválido")
	}
	if cmd.nome == "criar-superadmin" && (len([]rune(cmd.nomeAdmin)) < 2 || len([]rune(cmd.nomeAdmin)) > 100) {
		return nil, errors.New("--nome é obrigatório (2 a 100 caracteres)")
	}
	return cmd, nil
}

// LeitorPadrao lê do terminal sem eco (term.ReadPassword) ou, sem TTY, uma linha do stdin.
// Os prompts vão para stderr (não poluem a saída de scripts).
func LeitorPadrao(in *os.File, prompts io.Writer) LeitorSenha {
	fd := int(in.Fd())
	if term.IsTerminal(fd) {
		return func(prompt string) (string, error) {
			fmt.Fprint(prompts, prompt)
			b, err := term.ReadPassword(fd)
			fmt.Fprintln(prompts)
			return string(b), err
		}
	}
	return LeitorDeTexto(in)
}

// LeitorDeTexto lê uma senha por linha de r (uso sem TTY: pipes e testes).
func LeitorDeTexto(r io.Reader) LeitorSenha {
	br := bufio.NewReader(r)
	return func(string) (string, error) {
		linha, err := br.ReadString('\n')
		if err != nil && !(errors.Is(err, io.EOF) && linha != "") {
			return "", errors.New("não foi possível ler a senha da entrada padrão")
		}
		return strings.TrimRight(linha, "\r\n"), nil
	}
}

// lerSenhaConfirmada pede a senha duas vezes e confere tamanho e igualdade.
func lerSenhaConfirmada(ler LeitorSenha) (string, error) {
	s1, err := ler("Senha (mínimo 12 caracteres): ")
	if err != nil {
		return "", err
	}
	if len([]rune(s1)) < services.SenhaAdminMinima {
		return "", fmt.Errorf("a senha deve ter pelo menos %d caracteres", services.SenhaAdminMinima)
	}
	if len(s1) > 72 {
		return "", errors.New("a senha deve ter no máximo 72 bytes")
	}
	s2, err := ler("Confirme a senha: ")
	if err != nil {
		return "", err
	}
	if s1 != s2 {
		return "", errors.New("as senhas não conferem")
	}
	return s1, nil
}

// Executar roda o comando admin sobre o banco (já migrado). Mensagens de resultado vão para `saida`.
func Executar(args []string, db *gorm.DB, ler LeitorSenha, saida io.Writer) error {
	cmd, err := parse(args)
	if err != nil {
		return err
	}
	admins := repositories.NovoAdminRepository(db)
	auditoria := repositories.NovoAuditoriaRepository(db)
	existente, err := admins.BuscarPorEmail(cmd.email)
	if err != nil {
		return err
	}
	switch cmd.nome {
	case "criar-superadmin":
		if existente != nil {
			return fmt.Errorf("já existe um administrador com o e-mail %s (use: admin resetar-senha)", cmd.email)
		}
	default:
		if existente == nil {
			return fmt.Errorf("nenhum administrador com o e-mail %s", cmd.email)
		}
	}
	senha, err := lerSenhaConfirmada(ler)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if cmd.nome == "criar-superadmin" {
		a := &models.Administrador{Nome: cmd.nomeAdmin, Email: cmd.email, SenhaHash: string(hash),
			Papel: models.PapelSuperadmin, Ativo: true}
		err = db.Transaction(func(tx *gorm.DB) error {
			if err := admins.Criar(tx, a); err != nil {
				return err
			}
			return services.RegistrarAuditoria(auditoria, tx, a.ID, "cli", services.AcaoCLICriarSuperadmin,
				services.AlvoAdmin, a.ID, "Superadmin criado pela CLI", map[string]any{"email": cmd.email})
		})
		if err != nil {
			if repositories.EhDuplicata(err) {
				return fmt.Errorf("já existe um administrador com o e-mail %s", cmd.email)
			}
			return err
		}
		fmt.Fprintf(saida, "Superadmin %s criado (id %d).\n", cmd.email, a.ID)
		return nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := admins.AlterarSenha(tx, existente.ID, string(hash)); err != nil {
			return err
		}
		return services.RegistrarAuditoria(auditoria, tx, existente.ID, "cli", services.AcaoCLIResetarSenha,
			services.AlvoAdmin, existente.ID, "Senha redefinida pela CLI", map[string]any{"email": cmd.email})
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(saida, "Senha de %s redefinida; as sessões abertas foram encerradas.\n", cmd.email)
	return nil
}
