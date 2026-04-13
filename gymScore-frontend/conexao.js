const mysql = require('mysql2/promise');  // Usar a versão promise
const dotenv = require('dotenv');

// Carregar variáveis do arquivo .env
dotenv.config();

// Configuração do pool de conexões
const pool = mysql.createPool({
    host: process.env.DB_HOST,
    port: process.env.DB_PORT,
    user: process.env.DB_USER,
    password: process.env.DB_PASSWORD,
    database: process.env.DB_NAME,
    waitForConnections: true,
    connectionLimit: 10,
    queueLimit: 0, 
});

// Função assíncrona para abrir a conexão
async function openConnection() {
    try {
        const connection = await pool.getConnection();
        console.log('Conexão bem-sucedida ao banco de dados!');
        connection.release();  // Liberar a conexão após o uso
    } catch (err) {
        console.error('Erro ao conectar ao banco de dados:', err);
        throw err;  // Propagar o erro caso a conexão falhe
    }
}

// Função assíncrona para fechar a conexão
async function closeConnection() {
    try {
        await pool.end();  // Fechar todas as conexões do pool
        console.log('Conexão com o banco de dados fechada com sucesso!');
    } catch (err) {
        console.error('Erro ao fechar a conexão com o banco de dados:', err);
    }
}

module.exports = {
    pool,  // Exportando o pool de conexões
    openConnection,
    closeConnection
};
