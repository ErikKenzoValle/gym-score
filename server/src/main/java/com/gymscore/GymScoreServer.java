package com.gymscore;

import java.io.BufferedReader;
import java.io.BufferedWriter;
import java.io.InputStreamReader;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.net.ServerSocket;
import java.net.Socket;

public class GymScoreServer {
    private static final int PORT = 8080;

    public static void main(String[] args) {
        try (ServerSocket serverSocket = new ServerSocket(PORT)) {
            System.out.println("Servidor iniciado na porta " + PORT);

            while (true) {
                Socket clientSocket = serverSocket.accept();
                System.out.println("Cliente conectado: " + clientSocket.getInetAddress().getHostAddress());

                // Processar requisição em uma nova thread
                new Thread(() -> handleClient(clientSocket)).start();
            }
        } catch (Exception e) {
            System.err.println("Erro no servidor: " + e.getMessage());
        }
    }

    private static void handleClient(Socket clientSocket) {
        try (
                BufferedReader in = new BufferedReader(new InputStreamReader(clientSocket.getInputStream()));
                PrintWriter out = new PrintWriter(new BufferedWriter(new OutputStreamWriter(clientSocket.getOutputStream())), true)
        ) {
            // Ler a requisição do cliente
            String requestLine = in.readLine();
            System.out.println("Requisição recebida: " + requestLine);

            // Construir resposta simples
            String response = "HTTP/1.1 200 OK\r\n" +
                    "Content-Type: text/plain\r\n" +
                    "\r\n" +
                    "Bem-vindo ao Gym Score!";

            // Enviar resposta ao cliente
            out.write(response);
            out.flush();
        } catch (Exception e) {
            System.err.println("Erro ao processar cliente: " + e.getMessage());
        } finally {
            try {
                clientSocket.close();
            } catch (Exception e) {
                System.err.println("Erro ao fechar conexão: " + e.getMessage());
            }
        }
    }
}
