package com.radael.condoguard.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.security.authentication.AuthenticationManager;
import org.springframework.security.authentication.BadCredentialsException;
import org.springframework.security.authentication.UsernamePasswordAuthenticationToken;
import org.springframework.security.core.AuthenticationException;
import org.springframework.security.core.userdetails.UsernameNotFoundException;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.web.bind.annotation.*;

import com.radael.condoguard.dto.AuthRequest;
import com.radael.condoguard.model.User;
import com.radael.condoguard.repository.UserRepository;
import com.radael.condoguard.security.JwtUtil;

import java.util.Map;
import java.util.HashMap;

@RestController
@RequestMapping("/auth")
public class AuthController {

    @Autowired
    private AuthenticationManager authenticationManager;

    @Autowired
    private JwtUtil jwtUtil;

    @Autowired
    private PasswordEncoder passwordEncoder;

    @Autowired
    private UserRepository userRepository;

    @PostMapping("/register")
    public ResponseEntity<Map<String, String>> registerUser(@RequestBody User user) {
        // Codifica a senha antes de salvar o usuário
        user.setPassword(passwordEncoder.encode(user.getPassword()));
        userRepository.save(user);

        // Cria o corpo da resposta
        Map<String, String> response = new HashMap<>();
        response.put("message", "User registered successfully!");

        return ResponseEntity.status(HttpStatus.CREATED).body(response);
    }

    @PostMapping("/login")
    public ResponseEntity<Map<String, String>> login(@RequestBody AuthRequest authRequest) {
        try {
            // O AuthenticationManager tenta autenticar as credenciais fornecidas
            authenticationManager.authenticate(
                new UsernamePasswordAuthenticationToken(authRequest.getUsername(), authRequest.getPassword())
            );
        } catch (UsernameNotFoundException e) {
            // Captura exceções quando o usuário não é encontrado
            Map<String, String> errorResponse = new HashMap<>();
            errorResponse.put("error", "Incorrect username or password");
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(errorResponse);
        } catch (BadCredentialsException e) {
            // Captura exceções quando as credenciais estão incorretas
            Map<String, String> errorResponse = new HashMap<>();
            errorResponse.put("error", "Incorrect username or password");
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(errorResponse);
        } catch (AuthenticationException e) {
            // Captura todas as outras exceções de autenticação
            Map<String, String> errorResponse = new HashMap<>();
            errorResponse.put("error", "Authentication failed");
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED).body(errorResponse);
        }

        // Gera um token JWT após autenticação bem-sucedida
        String token = jwtUtil.generateToken(authRequest.getUsername());

        // Cria o corpo da resposta
        Map<String, String> response = new HashMap<>();
        response.put("token", token);

        return ResponseEntity.ok(response);
    }
}

