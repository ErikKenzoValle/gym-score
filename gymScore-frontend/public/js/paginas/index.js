// Script da página index.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Sessão no cookie HttpOnly: o cache local só diz se há alguém logado (a API confirma depois)
var _usuarioLocal = null;
try { _usuarioLocal = localStorage.getItem('gym_user'); } catch (e) {}
window.location.href = _usuarioLocal ? '/menu' : '/login';
