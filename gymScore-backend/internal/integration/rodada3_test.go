//go:build integration

package integration

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"gymScore-backend/internal/config"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ─── #8 Troca de senha atômica ───────────────────────────────────────────────

// 10 trocas simultâneas com o MESMO token: no máximo uma vence; as demais perdem
// (401 se perderam a corrida no UPDATE, 400 se já enxergaram a senha nova).
func TestSenha_TrocasSimultaneasComMesmoTokenSoUmaVence(t *testing.T) {
	for rodada := 0; rodada < 3; rodada++ {
		u := novoUsuario(t)
		const n = 10
		codigos := make([]int, n)
		tokens := make([]string, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				r := api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{
					"senha_atual": u.Senha, "nova_senha": fmt.Sprintf("nova-senha-%02d-xyz", i),
				})
				codigos[i] = r.Status
				if r.Status == 200 {
					tokens[i], _ = r.data()["token"].(string)
				}
			}(i)
		}
		wg.Wait()

		venceu := -1
		for i, c := range codigos {
			switch c {
			case 200:
				if venceu != -1 {
					t.Fatalf("mais de uma troca venceu: %v", codigos)
				}
				venceu = i
			case 401, 400:
			default:
				t.Errorf("status inesperado %d", c)
			}
		}
		if venceu == -1 {
			t.Fatalf("nenhuma troca venceu: %v", codigos)
		}
		var v int
		db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", u.ID).Scan(&v)
		if v != 1 {
			t.Fatalf("versão deveria ter subido exatamente 1 vez, está em %d", v)
		}
		// só a senha do vencedor vale; token antigo morto, token novo vivo
		esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": fmt.Sprintf("nova-senha-%02d-xyz", venceu)}), 200)
		for i := range codigos {
			if i != venceu {
				esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": fmt.Sprintf("nova-senha-%02d-xyz", i)}), 401)
			}
		}
		esperar(t, api(t, "GET", "/api/carteira", u.Token, nil), 401)
		esperar(t, api(t, "GET", "/api/carteira", tokens[venceu], nil), 200)
	}
}

func TestSenha_TrocaComTokenJaRevogadoDa401(t *testing.T) {
	u := novoUsuario(t)
	esperar(t, api(t, "POST", "/api/logout", u.Token, nil), 200)
	esperar(t, api(t, "PATCH", "/api/usuarios/senha", u.Token, map[string]any{"senha_atual": u.Senha, "nova_senha": "nova-senha-123"}), 401)
	esperar(t, api(t, "POST", "/api/login", "", map[string]any{"email": u.Email, "senha": u.Senha}), 200) // senha não mudou
}

// ─── #9 Fuso da sessão MySQL == fuso do Go ───────────────────────────────────

// dsnComFuso devolve o DSN do banco de teste com `loc=<nome>` (e, se aplicar, time_zone da sessão).
func dsnComFuso(nome string, loc *time.Location, aplicar bool) string {
	base := strings.Replace(dsnAdmin, "/?", "/"+nomeBanco+"?", 1)
	base = strings.Replace(base, "loc=Local", "loc="+url.QueryEscape(nome), 1)
	if aplicar {
		return config.AplicarFusoDSN(base, loc)
	}
	return base
}

func TestFuso_DefaultsDoMySQLBatemComAHoraDoGo(t *testing.T) {
	// o container de teste roda em UTC; o Go roda em fusos distintos
	for _, nome := range []string{"America/Sao_Paulo", "Asia/Tokyo", "UTC"} {
		loc, err := time.LoadLocation(nome)
		if err != nil {
			t.Skipf("tzdata indisponível para %s", nome) // só se o SO não tiver tzdata
		}
		abrir := func(aplicar bool) *gorm.DB {
			d, err := gorm.Open(mysql.Open(dsnComFuso(nome, loc, aplicar)), &gorm.Config{Logger: logger.Discard})
			if err != nil {
				t.Fatal(err)
			}
			return d
		}
		tabela := "tz_teste_" + strings.NewReplacer("/", "_", "-", "_").Replace(strings.ToLower(nome))
		db.Exec("DROP TABLE IF EXISTS " + tabela)
		db.Exec("CREATE TABLE " + tabela + " (id INT AUTO_INCREMENT PRIMARY KEY, criado DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), agora DATETIME(3) NULL)")
		defer db.Exec("DROP TABLE IF EXISTS " + tabela)

		diferenca := func(d *gorm.DB) time.Duration {
			if err := d.Exec("INSERT INTO " + tabela + " (agora) VALUES (NOW(3))").Error; err != nil {
				t.Fatal(err)
			}
			var r struct{ Criado, Agora time.Time }
			if err := d.Raw("SELECT criado, agora FROM " + tabela + " ORDER BY id DESC LIMIT 1").Scan(&r).Error; err != nil {
				t.Fatal(err)
			}
			if dd := r.Criado.Sub(r.Agora); dd > time.Second || dd < -time.Second {
				t.Errorf("%s: DEFAULT e NOW() divergem entre si (%v vs %v)", nome, r.Criado, r.Agora)
			}
			return time.Since(r.Criado)
		}

		// com o parâmetro: DEFAULT/NOW() lidos pelo Go batem com a hora local
		if d := diferenca(abrir(true)); d < -5*time.Second || d > 5*time.Second {
			t.Errorf("%s: com time_zone aplicado o horário deveria bater, diferença %v", nome, d)
		}
		// controle: sem o parâmetro e com fuso != UTC, o deslocamento aparece (prova que o teste detecta o bug)
		_, off := time.Now().In(loc).Zone()
		if off != 0 {
			if d := diferenca(abrir(false)); d > -time.Hour && d < time.Hour {
				t.Errorf("%s: sem time_zone esperava deslocamento de ~%ds, diferença %v", nome, off, d)
			}
		}
	}
}

func TestFuso_SessaoDoAppUsaOFusoDoGo(t *testing.T) {
	var sessao string
	db.Raw("SELECT @@session.time_zone").Scan(&sessao)
	esperado := config.FusoMySQL(time.Local, time.Now())
	if sessao != esperado {
		t.Errorf("time_zone da sessão = %q, esperado %q (fuso do Go)", sessao, esperado)
	}
	// linha criada pelo app via DEFAULT do banco (desafio_votos.criado_em não passa pelo Go no INSERT cru)
	a, b := usuarioComSaldo(t, 3000), usuarioComSaldo(t, 3000)
	id := criarDesafio(t, a, 1000, 2)
	esperar(t, acao(t, b, id, "entrar", nil), 200)
	esperar(t, acao(t, a, id, "iniciar", nil), 200)
	esperar(t, acao(t, a, id, "abrir-votacao", nil), 200)
	if err := db.Exec("INSERT INTO desafio_votos (id_desafio, id_eleitor, id_votado) VALUES (?, ?, ?)", id, a.ID, b.ID).Error; err != nil {
		t.Fatal(err)
	}
	var criado time.Time
	db.Raw("SELECT criado_em FROM desafio_votos WHERE id_desafio = ?", id).Scan(&criado)
	if d := time.Since(criado); d < -5*time.Second || d > 30*time.Second {
		t.Errorf("criado_em por DEFAULT deslocado: %v (agora %v)", criado, time.Now())
	}
}
