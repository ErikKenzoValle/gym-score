package services_test

import (
	"errors"
	"net/http"
	"testing"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/services"
	"gymScore-backend/pkg/utils"
)

func reqUsuario() *models.CriarUsuarioRequest {
	return &models.CriarUsuarioRequest{
		Nome: "João", Sobrenome: "Silva", Email: "joao@example.com", Senha: "senha12345",
		CPF: "529.982.247-25", DataNascimento: "1995-03-15", Genero: "M",
	}
}

func statusDe(t *testing.T, err error) int {
	t.Helper()
	e, ok := services.AsErroNegocio(err)
	if !ok {
		t.Fatalf("esperava ErroNegocio, obteve %T: %v", err, err)
	}
	return e.Status
}

func TestCriarUsuario_Sucesso(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	resp, err := svc.CriarUsuario(reqUsuario())
	if err != nil {
		t.Fatalf("esperava sucesso: %v", err)
	}
	if resp.Email != "joao@example.com" || resp.ID == 0 || resp.Nome != "João" {
		t.Errorf("resposta inesperada: %+v", resp)
	}
}

func TestCriarUsuario_SenhaNaoFicaEmTextoPuro(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	if _, err := svc.CriarUsuario(reqUsuario()); err != nil {
		t.Fatal(err)
	}
	if repo.usuarios[1].Senha == "senha12345" || len(repo.usuarios[1].Senha) < 50 {
		t.Errorf("senha deveria estar hasheada (bcrypt): %q", repo.usuarios[1].Senha)
	}
}

func TestCriarUsuario_Invalidos(t *testing.T) {
	casos := map[string]func(r *models.CriarUsuarioRequest){
		"email inválido": func(r *models.CriarUsuarioRequest) { r.Email = "email-invalido" },
		"cpf curto":      func(r *models.CriarUsuarioRequest) { r.CPF = "52998224725" },
		"cpf inválido":   func(r *models.CriarUsuarioRequest) { r.CPF = "529.982.247-26" },
		"cpf com letras": func(r *models.CriarUsuarioRequest) { r.CPF = "529.982.247-2a" },
	}
	for nome, mut := range casos {
		svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
		r := reqUsuario()
		mut(r)
		_, err := svc.CriarUsuario(r)
		if err == nil || statusDe(t, err) != http.StatusBadRequest {
			t.Errorf("%s: esperava 400, obteve %v", nome, err)
		}
	}
}

func TestCriarUsuario_Duplicados(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	if _, err := svc.CriarUsuario(reqUsuario()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CriarUsuario(reqUsuario())
	if err == nil || statusDe(t, err) != http.StatusConflict {
		t.Errorf("e-mail duplicado deveria ser 409: %v", err)
	}
	r := reqUsuario()
	r.Email = "outro@example.com"
	_, err = svc.CriarUsuario(r)
	if err == nil || statusDe(t, err) != http.StatusConflict || err.Error() != "CPF já cadastrado" {
		t.Errorf("CPF duplicado deveria ser 409: %v", err)
	}
}

func TestLogin(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	if _, err := svc.CriarUsuario(reqUsuario()); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.Login(&models.LoginRequest{Email: "joao@example.com", Senha: "senha12345"}, "segredo", nil)
	if err != nil || resp.Token == "" {
		t.Fatalf("login válido falhou: %v", err)
	}
	for _, l := range []models.LoginRequest{
		{Email: "joao@example.com", Senha: "errada"},
		{Email: "naoexiste@example.com", Senha: "senha12345"},
	} {
		_, err := svc.Login(&l, "segredo", nil)
		if err == nil || statusDe(t, err) != http.StatusUnauthorized || err.Error() != "usuário ou senha inválidos" {
			t.Errorf("credenciais ruins deveriam dar 401 com mensagem única: %v", err)
		}
	}
}

func TestPublico_NaoExpoeDadosSensiveis(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	pub, err := svc.Publico(u.ID)
	if err != nil || pub.Nome != "João" {
		t.Fatalf("público: %v %+v", err, pub)
	}
	if _, err := svc.Publico(999); err == nil || statusDe(t, err) != http.StatusNotFound {
		t.Errorf("inexistente deveria ser 404: %v", err)
	}
}

func TestBuscar_ExigeDoisCaracteres(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	for _, q := range []string{"", " a ", "a"} {
		if _, err := svc.Buscar(1, q); err == nil || statusDe(t, err) != http.StatusBadRequest {
			t.Errorf("q=%q deveria dar 400: %v", q, err)
		}
	}
	if _, err := svc.Buscar(1, "ab"); err != nil {
		t.Errorf("q=ab deveria ser aceito: %v", err)
	}
}

func TestAlterarSenha_Regras(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	if _, err := svc.AlterarSenha(u.ID, 0, "", &models.AlterarSenhaRequest{SenhaAtual: "errada", NovaSenha: "nova-senha-123"}, "s"); err == nil {
		t.Error("senha atual errada deveria falhar")
	}
	if _, err := svc.AlterarSenha(u.ID, 0, "", &models.AlterarSenhaRequest{SenhaAtual: "senha12345", NovaSenha: "curta"}, "s"); err == nil {
		t.Error("nova senha < 8 deveria falhar")
	}
	tok, err := svc.AlterarSenha(u.ID, 0, "", &models.AlterarSenhaRequest{SenhaAtual: "senha12345", NovaSenha: "nova-senha-123"}, "s")
	if err != nil {
		t.Errorf("troca válida falhou: %v", err)
	}
	if claims, err := utils.ValidarToken(tok, "s"); err != nil || claims.Versao != 1 {
		t.Errorf("o novo token deveria carregar a versão 1: %+v %v", claims, err)
	}
	if _, err := svc.Login(&models.LoginRequest{Email: "joao@example.com", Senha: "nova-senha-123"}, "s", nil); err != nil {
		t.Errorf("login com a nova senha falhou: %v", err)
	}
}

func TestErroNegocio_ComEnvolucro(t *testing.T) {
	base := &services.ErroNegocio{Status: 418, Mensagem: "chá"}
	if e, ok := services.AsErroNegocio(errors.Join(errors.New("x"), base)); !ok || e.Status != 418 {
		t.Error("deveria extrair ErroNegocio de erro composto")
	}
	if _, ok := services.AsErroNegocio(errors.New("erro de banco com detalhes internos")); ok {
		t.Error("erro comum não é erro de negócio")
	}
}

func TestLogout_SoRevogaAVersaoApresentada(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	if err := svc.Logout(u.ID, 0); err != nil || repo.usuarios[u.ID].TokenVersao != 1 {
		t.Fatalf("logout deveria incrementar para 1: %v", err)
	}
	// um token antigo (versão 0) tentando "logout" não pode derrubar a sessão atual (versão 1)
	_ = svc.Logout(u.ID, 0)
	if repo.usuarios[u.ID].TokenVersao != 1 {
		t.Errorf("token já revogado não pode incrementar de novo: %d", repo.usuarios[u.ID].TokenVersao)
	}
}

func TestLogin_TokenCarregaVersaoAtual(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	repo.usuarios[u.ID].TokenVersao = 4
	resp, _ := svc.Login(&models.LoginRequest{Email: "joao@example.com", Senha: "senha12345"}, "s", nil)
	if c, err := utils.ValidarToken(resp.Token, "s"); err != nil || c.Versao != 4 {
		t.Errorf("claim tv deveria ser 4: %+v %v", c, err)
	}
}

func TestAlterarSenha_TokenJaRevogadoDa401(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	req := &models.AlterarSenhaRequest{SenhaAtual: "senha12345", NovaSenha: "nova-senha-123"}
	if _, err := svc.AlterarSenha(u.ID, 0, "", req, "s"); err != nil {
		t.Fatal(err)
	}
	// segunda troca com o MESMO token (versão 0, já revogada): perde
	req2 := &models.AlterarSenhaRequest{SenhaAtual: "senha12345", NovaSenha: "outra-senha-456"}
	if _, err := svc.AlterarSenha(u.ID, 0, "", req2, "s"); err == nil {
		t.Fatal("deveria falhar")
	}
}

// ─── Modos da conta (rodada 4) ──────────────────────────────────────────────

func TestTrocarModo_ConfereNoBanco(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	repo.usuarios[u.ID].TokenVersao = 3

	// Sem pedido de instrutor: só atleta.
	if _, err := svc.TrocarModo(u.ID, 3, "instrutor", "s"); err == nil || statusDe(t, err) != http.StatusForbidden {
		t.Fatalf("sem pedido deveria dar 403: %v", err)
	} else if e, _ := services.AsErroNegocio(err); e.Codigo != services.CodigoModoIndisponivel {
		t.Errorf("código esperado modo_indisponivel: %+v", e)
	}
	for _, st := range []string{models.CadastroPendente, models.CadastroRejeitado} {
		repo.instrutores = map[uint]string{u.ID: st}
		_, err := svc.TrocarModo(u.ID, 3, "instrutor", "s")
		if err == nil || statusDe(t, err) != http.StatusForbidden {
			t.Errorf("pedido %s não libera o modo instrutor: %v", st, err)
		}
	}
	if _, err := svc.TrocarModo(u.ID, 3, "admin", "s"); err == nil || statusDe(t, err) != http.StatusBadRequest {
		t.Errorf("modo desconhecido deveria dar 400: %v", err)
	}

	repo.instrutores = map[uint]string{u.ID: models.CadastroAprovado}
	resp, err := svc.TrocarModo(u.ID, 3, "instrutor", "s")
	if err != nil {
		t.Fatal(err)
	}
	c, err := utils.ValidarToken(resp.Token, "s")
	if err != nil || c.Modo != "instrutor" || c.Versao != 3 || c.UserID != u.ID {
		t.Fatalf("token novo deveria ter modo=instrutor e a MESMA versão: %+v %v", c, err)
	}
	if resp.Usuario.ModoAtivo != "instrutor" || len(resp.Usuario.ModosDisponiveis) != 2 ||
		resp.Usuario.Perfil != "atleta" || resp.Usuario.Instrutor == nil || resp.Usuario.Instrutor.Status != "aprovada" {
		t.Errorf("sessão devolvida: %+v", resp.Usuario.SessaoModos)
	}
	// Volta para atleta.
	resp, err = svc.TrocarModo(u.ID, 3, "atleta", "s")
	if c, _ := utils.ValidarToken(resp.Token, "s"); err != nil || c.Modo != "atleta" {
		t.Errorf("troca para atleta: %+v %v", c, err)
	}
}

func TestTrocarModo_AcademiaNaoTemModos(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	a := &models.Usuario{Nome: "Acad", Email: "a@x.com", Perfil: models.PerfilAcademia}
	_ = repo.Criar(a)
	if _, err := svc.TrocarModo(a.ID, 0, "atleta", "s"); err == nil || statusDe(t, err) != http.StatusForbidden {
		t.Errorf("academia não troca de modo: %v", err)
	}
	p, err := svc.Perfil(a.ID, "")
	if err != nil || p.Perfil != "academia" || p.ModoAtivo != "" || p.ModosDisponiveis == nil || len(p.ModosDisponiveis) != 0 || p.Instrutor != nil {
		t.Errorf("perfil da academia: %+v %v", p.SessaoModos, err)
	}
}

func TestLogin_EntraNoUltimoModoSePermitido(t *testing.T) {
	repo := novoMockUsuarioRepo()
	svc := services.NovoUsuarioService(repo, nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	req := &models.LoginRequest{Email: "joao@example.com", Senha: "senha12345"}
	pref := func(modo string) func(uint) string {
		return func(id uint) string {
			if id != u.ID {
				return ""
			}
			return modo
		}
	}
	modoDe := func(r *models.LoginResponse) string {
		c, err := utils.ValidarToken(r.Token, "s")
		if err != nil {
			t.Fatal(err)
		}
		return c.Modo
	}
	// Sem preferência / preferência indisponível (pedido pendente): atleta.
	r, _ := svc.Login(req, "s", nil)
	if modoDe(r) != "atleta" || r.Usuario.ModoAtivo != "atleta" || r.Usuario.Instrutor != nil {
		t.Errorf("login sem preferência: %s %+v", modoDe(r), r.Usuario.SessaoModos)
	}
	repo.instrutores = map[uint]string{u.ID: models.CadastroRejeitado}
	r, _ = svc.Login(req, "s", pref("instrutor"))
	if modoDe(r) != "atleta" || r.Usuario.Instrutor == nil || r.Usuario.Instrutor.Status != "rejeitada" ||
		r.Usuario.Instrutor.MotivoRejeicao == nil {
		t.Errorf("preferência instrutor com pedido rejeitado: %s %+v", modoDe(r), r.Usuario.SessaoModos)
	}
	repo.instrutores = map[uint]string{u.ID: models.CadastroAprovado}
	r, _ = svc.Login(req, "s", pref("instrutor"))
	if modoDe(r) != "instrutor" || r.Modo != "instrutor" || r.Usuario.ModoAtivo != "instrutor" {
		t.Errorf("preferência instrutor aprovada: %s %+v", modoDe(r), r.Usuario.SessaoModos)
	}
	r, _ = svc.Login(req, "s", pref("hacker"))
	if modoDe(r) != "atleta" {
		t.Errorf("preferência inválida deveria cair no atleta: %s", modoDe(r))
	}
}

func TestAlterarSenha_MantemOModo(t *testing.T) {
	svc := services.NovoUsuarioService(novoMockUsuarioRepo(), nil)
	u, _ := svc.CriarUsuario(reqUsuario())
	tok, err := svc.AlterarSenha(u.ID, 0, "instrutor", &models.AlterarSenhaRequest{SenhaAtual: "senha12345", NovaSenha: "nova-senha-123"}, "s")
	if c, _ := utils.ValidarToken(tok, "s"); err != nil || c.Modo != "instrutor" {
		t.Errorf("a troca de senha deveria manter o modo: %+v %v", c, err)
	}
}
