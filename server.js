const express = require('express');
const bodyParser = require('body-parser');
const conexao = require('./conexao'); // Importando o arquivo de conexão com o banco
const path = require('path');

const app = express();
app.use(bodyParser.json());

app.use(express.static(path.join(__dirname, 'public')));

app.get("/", (req, res) => {
    const filePath = path.join(__dirname, "public", "index.html");
    res.sendFile(filePath);
});

// Adicionar um novo usuário
app.post("/api/usuarios", async (req, res) => {
    const { nome, sobrenome, email, senha, data_nascimento, genero } = req.body;

    const sql = "CALL criar_usuario(?, ?, ?, ?, ?, ?)"; // Nome da procedure

    let connection;
    try {
        connection = await conexao.pool.getConnection(); // Pegando uma conexão do pool
        
        // Executando a procedure e passando os parâmetros
        const [result] = await connection.execute(sql, [nome, sobrenome, email, senha, data_nascimento, genero]);

        // Retorna a resposta para o cliente
        res.status(201).json({ id: result.insertId, nome, sobrenome, email });
    } catch (err) {
        console.error('Erro ao adicionar usuário:', err);
        res.status(500).send('Erro ao adicionar usuário');
    } finally {
        if (connection) {
            connection.release(); // Libera a conexão após a execução
        }
    }
});

// Validar login
app.post("/api/login", async (req, res) => {
    const { email, senha } = req.body;

    const sql = "SELECT validar_login(?, ?) AS autenticado";

    let connection;
    try {
        connection = await conexao.pool.getConnection(); // Pegando uma conexão do pool
        
        // Executa a função 'validar_login' com os parâmetros fornecidos
        const [results] = await connection.execute(sql, [email, senha]);

        // Verifica o valor retornado pela função 'validar_login'
        if (results.length > 0 && results[0].autenticado) {
            res.status(200).json({
                message: "Usuário autenticado com sucesso!",
                autenticado: results[0].autenticado
            });
        } else {
            res.status(401).json({
                message: "Usuário ou senha inválidos",
                autenticado: results[0].autenticado
            });
        }
    } catch (err) {
        console.error('Erro ao autenticar usuário:', err);
        res.status(500).send('Erro ao autenticar usuário');
    } finally {
        if (connection) {
            connection.release();
        }
    }
});


const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
    console.log(`Servidor está rodando na porta ${PORT}`);
});
