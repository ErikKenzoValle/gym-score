// Script da página privacidade.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Não mostra banner aqui — o usuário já está lendo a política
// Aceitar via botão da própria página

// Eventos (antes eram atributos on* no HTML)
document.getElementById('linkVoltar').addEventListener('click', function(e) {
    if (history.length > 1) { e.preventDefault(); history.back(); }
});
document.getElementById('btnAceitarLgpd').addEventListener('click', function() {
    acceptLGPD();
    window.location.href = '/login';
});
