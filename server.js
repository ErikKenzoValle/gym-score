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
        // Define os campos esperados no CSV
        const fields = ["nome", "sobrenome", "email", "senha", "data_nascimento", "genero"];

        const csvData = createCSV({ nome, sobrenome, email, senha, data_nascimento, genero }, fields);

        const javaResponse = await axios.post(
            "http://localhost:8080/validar-email",
            csvData,
            { headers: { "Content-Type": "text/csv" } }
        );

        if (!javaResponse.data.valid) {
            return res.status(400).json({ message: "E-mail inválido" });
        }

        const sql = "CALL criar_usuario(?, ?, ?, ?, ?, ?)";
        let connection = await conexao.pool.getConnection();
        const [results] = await connection.execute(sql, [nome, sobrenome, email, senha, data_nascimento, genero]);

        res.status(201).json({ id: results.insertId, nome, sobrenome, email });
    } catch (err) {
        console.error("Erro ao adicionar usuário:", err);
        res.status(500).send("Erro ao adicionar usuário");
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

// Perfil de um usuário
app.get("/api/usuarios/:id", async (req, res) => {
    const { id } = req.params;
    
    const sql = "CALL ver_usuario(?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        const [results] = await connection.execute(sql, [id]);

        if (results && results[0]) {
            res.status(200).json(results[0]);
        } else {
            res.status(404).send("Usuário não encontrado.");
        }
    } catch (err) {
        console.error("Erro ao buscar usuário:", err);
        res.status(500).send(`Erro ao buscar usuário: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Lista de desafios
app.get("/api/desafios/view", async (req, res) => {
    const sql = "SELECT * FROM desafios"; // Consulta para pegar todos os desafios

    let connection;
    try {
        connection = await conexao.pool.getConnection();

        const [results] = await connection.execute(sql);

        if (results && results.length > 0) {
            res.status(200).json(results);
        } else {
            res.status(404).send("Nenhum desafio encontrado.");
        }
    } catch (err) {
        console.error("Erro ao buscar desafios:", err);
        res.status(500).send(`Erro ao buscar desafios: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Listar amigos de um usuário
app.post("/api/amigos/:id", async (req, res) => {
    const { id } = req.params;

    const sql = "CALL listar_amigos(?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        const [results] = await connection.execute(sql, [id]);

        res.status(200).json(results[0]);
    } catch (err) {
        console.error('Erro ao listar amigos:', err);
        res.status(500).send(`Erro ao listar amigos: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Adicionar um amigo
app.post("/api/amigos/adicionar", async (req, res) => {
    const { id_usuario, id_amigo } = req.body;

    const sql = "CALL adicionar_amigo(?, ?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        const [results] = await connection.execute(sql, [id_usuario, id_amigo]);

        res.status(200).json({
            message: "Amigo adicionado com sucesso!",
            id_usuario: id_usuario,
            id_amigo: id_amigo
        });
    } catch (err) {
        console.error('Erro ao adicionar amigo:', err);
        res.status(500).send(`Erro ao adicionar amigo: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Aceitar solicitação de amizade
app.post("/api/amigos/aceitar", async (req, res) => {
    const { id_usuario, id_amigo } = req.body;

    const sql = "CALL aceitar_amizade(?, ?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();

        // Chama a procedure diretamente
        await connection.execute(sql, [id_usuario, id_amigo]);

        res.status(200).json({
            message: "Amigo aceito com sucesso!",
            id_usuario: id_usuario,
            id_amigo: id_amigo
        });
    } catch (err) {
        console.error("Erro ao aceitar amigo:", err);
        res.status(500).send(`Erro ao aceitar amigo: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Listar desafios atuais de um usuário
app.get("/api/desafios/:id", async (req, res) => {
    const { id } = req.params;

    const sql = "CALL desafios_abertos(?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        const [results] = await connection.execute(sql, [id]);

        res.status(200).json(results[0]);
    } catch (err) {
        console.error('Erro ao listar desafios atuais:', err);
        res.status(500).send(`Erro ao listar desafios atuais: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Remover um amigo
app.post("/api/amigos/remover", async (req, res) => {
    const { id_usuario, id_amigo } = req.body;

    const sql = "CALL remover_amigo(?, ?)";

    let connection;
    try {
        connection = await conexao.pool.getConnection();
        
        const [results] = await connection.execute(sql, [id_usuario, id_amigo]);

        res.status(200).json({
            message: "Amigo removido com sucesso!",
            id_usuario: id_usuario,
            id_amigo: id_amigo
        });
    } catch (err) {
        console.error('Erro ao remover amigo:', err);
        res.status(500).send(`Erro ao remover amigo: ${err.message}`);
    } finally {
        if (connection) {
            connection.release();
        }
    }
});

// Função auxiliar para criar CSV manualmente
function createCSV(data, fields) {
    const csvLines = [fields.join(",")];
    const values = fields.map(field => data[field]);
    csvLines.push(values.join(","));
    return csvLines.join("\n");
}

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
    console.log(`Servidor está rodando na porta ${PORT}`);
});
