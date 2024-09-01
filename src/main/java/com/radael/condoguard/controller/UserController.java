package com.radael.condoguard.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.security.access.prepost.PreAuthorize;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.web.bind.annotation.*;

import com.radael.condoguard.model.User;
import com.radael.condoguard.repository.UserRepository;

import java.util.List;
import java.util.Optional;

@RestController
@RequestMapping("/users")
public class UserController {

    @Autowired
    private UserRepository userRepository;

    @Autowired
    private PasswordEncoder passwordEncoder;

    // Endpoint para listar todos os usuários (apenas para administradores)
    @GetMapping
    @PreAuthorize("hasRole('ADMIN')")
    public List<User> getAllUsers() {
        return userRepository.findAll();
    }

    // Endpoint para obter um usuário específico por ID (apenas para administradores)
    @GetMapping("/{id}")
    @PreAuthorize("hasRole('ADMIN')")
    public Optional<User> getUserById(@PathVariable String id) {
        return userRepository.findById(id);
    }

    // Endpoint para criar um novo usuário (apenas para administradores)
    @PostMapping
    @PreAuthorize("hasRole('ADMIN')")
    public String createUser(@RequestBody User user) {
        user.setPassword(passwordEncoder.encode(user.getPassword()));
        userRepository.save(user);
        return "User created successfully!";
    }

    // Endpoint para atualizar um usuário existente (apenas para administradores)
    @PutMapping("/{id}")
    @PreAuthorize("hasRole('ADMIN')")
    public String updateUser(@PathVariable String id, @RequestBody User userDetails) {
        Optional<User> userOptional = userRepository.findById(id);
        if (userOptional.isPresent()) {
            User user = userOptional.get();
            user.setUsername(userDetails.getUsername());
            user.setPassword(passwordEncoder.encode(userDetails.getPassword()));
            user.setRoles(userDetails.getRoles());
            userRepository.save(user);
            return "User updated successfully!";
        } else {
            return "User not found!";
        }
    }

    // Endpoint para deletar um usuário (apenas para administradores)
    @DeleteMapping("/{id}")
    @PreAuthorize("hasRole('ADMIN')")
    public String deleteUser(@PathVariable String id) {
        userRepository.deleteById(id);
        return "User deleted successfully!";
    }
}

