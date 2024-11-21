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

    try {

         // Chamada ao servidor Java para validar o e-mail
         const javaResponse = await axios.post('http://localhost:8080/validate-email', { email });

         if (!javaResponse.data.valid) {
             return res.status(400).json({ message: "E-mail inválido" });
         }

        const sql = "CALL criar_usuario(?, ?, ?, ?, ?, ?)"; // Nome da procedure

        let connection = await conexao.pool.getConnection(); // Pegando uma conexão do pool
        
        // Executando a procedure e passando os parâmetros
        const [results] = await connection.execute(sql, [nome, sobrenome, email, senha, data_nascimento, genero]);

        // Retorna a resposta para o cliente
        res.status(201).json({ id: results.insertId, nome, sobrenome, email });
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

// Criar um desafio
app.post("/api/desafios", async (req, res) => {
    const { titulo, id_criador, valor, descricao, local } = req.body;

    const sql = "CALL criar_desafio(?, ?, ?, ?, ?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        // Passando os parâmetros corretamente para a procedure
        const [results] = await connection.execute(sql, [titulo, id_criador, valor, descricao, local]);

        // Retorna a resposta com os dados do desafio criado
        res.status(201).json({ id: results.insertId, titulo, valor, descricao, local });
    } catch (err) {
        console.error('Erro ao adicionar desafio:', err);
        res.status(500).send('Erro ao adicionar desafio');
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Aceitar um desafio
app.post("/api/desafios/aceitar_desafio", async (req, res) => {
    const { id_desafio, id_usuario } = req.body;  // ID do desafio e do usuário, ambos do corpo da requisição

    const sql = "CALL aceitar_desafio(?, ?)";  // Chamada para a procedure com os parâmetros

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        // Executando a procedure com os parâmetros recebidos
        const [results] = await connection.execute(sql, [id_desafio, id_usuario]);

        // Retornando a resposta ao cliente com sucesso
        res.status(200).json({
            message: "Desafio aceito com sucesso!",
            id_desafio: id_desafio,
            id_desafiado: id_usuario
        });
    } catch (err) {
        console.error('Erro ao aceitar desafio:', err);
        
        // Retorna erro com a mensagem da procedure, se ocorrer
        res.status(500).send(`Erro ao aceitar desafio: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Iniciar um desafio
app.post("/api/desafios/iniciar", async (req, res) => {
    const { id_desafio } = req.body;  // ID do desafio que será iniciado

    const sql = "CALL iniciar_desafio(?)";  // Chama a procedure para iniciar o desafio

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        // Executa a procedure
        const [results] = await connection.execute(sql, [id_desafio]);

        // Retorna a resposta com sucesso
        res.status(200).json({
            message: "Desafio iniciado com sucesso!",
            id_desafio: id_desafio
        });
    } catch (err) {
        console.error('Erro ao iniciar desafio:', err);
        res.status(500).send(`Erro ao iniciar desafio: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Encerrar um desafio
app.post("/api/desafios/encerrar", async (req, res) => {
    const { id_desafio, id_vencedor, id_perdedor } = req.body;  // IDs do desafio, vencedor e perdedor

    const sql = "CALL encerrar_desafio(?, ?, ?)";  // Chama a procedure para encerrar o desafio

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        // Executa a procedure
        const [results] = await connection.execute(sql, [id_desafio, id_vencedor, id_perdedor]);

        // Retorna a resposta com sucesso
        res.status(200).json({
            message: "Desafio encerrado com sucesso!",
            id_desafio: id_desafio,
            vencedor: id_vencedor,
            perdedor: id_perdedor
        });
    } catch (err) {
        console.error('Erro ao encerrar desafio:', err);
        res.status(500).send(`Erro ao encerrar desafio: ${err.message}`);
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
