package services

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

// Mailer envia e-mails transacionais.
type Mailer interface {
	Enviar(para, assunto, corpo string) error
}

// SMTPMailer envia via SMTP (STARTTLS quando o servidor suporta, porta 587 típica).
type SMTPMailer struct {
	Host, Port, User, Pass, From string
}

// Enviar monta uma mensagem de texto simples e envia por SMTP.
func (m *SMTPMailer) Enviar(para, assunto, corpo string) error {
	// Impede injeção de cabeçalhos
	if strings.ContainsAny(para, "\r\n") || strings.ContainsAny(assunto, "\r\n") {
		return fmt.Errorf("cabeçalho de e-mail inválido")
	}
	msg := "From: " + m.From + "\r\n" +
		"To: " + para + "\r\n" +
		"Subject: " + assunto + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + corpo
	var auth smtp.Auth
	if m.User != "" {
		auth = smtp.PlainAuth("", m.User, m.Pass, m.Host)
	}
	return smtp.SendMail(m.Host+":"+m.Port, auth, m.From, []string{para}, []byte(msg))
}

// LogMailer é o modo demo (SMTP não configurado): escreve o e-mail no log do console.
type LogMailer struct{}

// Enviar apenas registra o conteúdo no log.
func (LogMailer) Enviar(para, assunto, corpo string) error {
	log.Printf("[MAIL-DEMO] SMTP não configurado. Para: %s | Assunto: %s\n%s", para, assunto, corpo)
	return nil
}

// NovoMailer escolhe SMTP se SMTP_HOST e SMTP_FROM estiverem definidos; senão modo demo.
func NovoMailer(host, port, user, pass, from string) Mailer {
	if host == "" || from == "" {
		return LogMailer{}
	}
	return &SMTPMailer{Host: host, Port: port, User: user, Pass: pass, From: from}
}
