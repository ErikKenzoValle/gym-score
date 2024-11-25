package com.gymscore;

import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.util.regex.Pattern;

public class GymScoreServer {
    private static final int PORT = 8080;

    public static void main(String[] args) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress(PORT), 0);

        // Validar se o email digitado está num formato válido
        server.createContext("/validar-email", exchange -> {
            if ("POST".equals(exchange.getRequestMethod())) {
                String body = new String(exchange.getRequestBody().readAllBytes());
                String email = body.split(":")[1].replaceAll("[^a-zA-Z0-9@._-]", "");
                boolean isValid = Pattern.compile("^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,6}$").matcher(email).matches();
                String response = "{\"valido\": " + isValid + "}";

                exchange.sendResponseHeaders(200, response.getBytes().length);

                OutputStream os = exchange.getResponseBody();
                os.write(response.getBytes());
                os.close();
            }
        });

        // Validar se o usuário tem saldo suficiente pra participar do desafio
        server.createContext("/validar-saldo", exchange -> {
            if ("POST".equals(exchange.getRequestMethod())) {
                try {
                    String body = new String(exchange.getRequestBody().readAllBytes());

                    String[] parts = body.split(",");
                    double saldoUsuario = Double.parseDouble(parts[0].split(":")[1].trim());
                    double valorDesafio = Double.parseDouble(parts[1].split(":")[1].trim());

                    boolean valido = saldoUsuario >= valorDesafio;

                    String response = "{\"Saldo valido\": " + valido + "}";
                    exchange.sendResponseHeaders(200, response.getBytes().length);

                    try (OutputStream os = exchange.getResponseBody()) {
                        os.write(response.getBytes());
                    }
                } catch (Exception e) {
                    e.printStackTrace();
                    String response = "{\"error\":\"Formato inválido da informação\"}";
                    exchange.sendResponseHeaders(400, response.getBytes().length);

                    try (OutputStream os = exchange.getResponseBody()) {
                        os.write(response.getBytes());
                    }
                }
            } else {
                // Caso o método HTTP não seja POST
                exchange.sendResponseHeaders(405, -1);
            }
        });

        // Finalizar desafio (atualizar saldos)
        server.createContext("/finalizar-desafio", exchange -> {
            if ("POST".equals(exchange.getRequestMethod())) {
                try {
                    String body = new String(exchange.getRequestBody().readAllBytes());

                    String[] parts = body.split(",");
                    double saldoVencedor = Double.parseDouble(parts[0].split(":")[1].trim());
                    double saldoPerdedor = Double.parseDouble(parts[1].split(":")[1].trim());
                    double valorApostado = Double.parseDouble(parts[2].split(":")[1].trim());

                    // Atualização dos saldos
                    saldoVencedor += valorApostado;
                    saldoPerdedor -= valorApostado;

                    String response = String.format("{\"Saldo vencedor\":%.2f,\"saldo perdedor\":%.2f}", saldoVencedor, saldoPerdedor);
                    exchange.sendResponseHeaders(200, response.getBytes().length);

                    try (OutputStream os = exchange.getResponseBody()) {
                        os.write(response.getBytes());
                    }
                } catch (Exception e) {
                    e.printStackTrace();
                    String response = "{\"error\":\"Formato inválido da informação\"}";
                    exchange.sendResponseHeaders(400, response.getBytes().length);

                    try (OutputStream os = exchange.getResponseBody()) {
                        os.write(response.getBytes());
                    }
                }
            } else {
                exchange.sendResponseHeaders(405, -1);
            }
        });

        server.start();
        System.out.println("Servidor Java rodando na porta " + PORT);
    }


}
