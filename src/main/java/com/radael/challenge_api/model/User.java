package com.radael.challenge_api.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

import java.util.HashSet;
import java.util.Objects;
import java.util.Set;

@Document(collection = "users")
public class User {
    @Id
    private String id;
    private String username;
    private String password; // Armazena o hash da senha
    private Set<String> roles = new HashSet<>(); // Ex: ["ROLE_USER", "ROLE_ADMIN"]

    // Construtor padrão
    public User() {}

    // Construtor com parâmetros
    public User(String username, String password, Set<String> roles) {
        this.username = username;
        this.setPassword(password); // Chama o setter para garantir o hash da senha
        this.roles = roles != null ? new HashSet<>(roles) : new HashSet<>();
    }

    // Getters e Setters
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getUsername() {
        return username;
    }

    public void setUsername(String username) {
        this.username = username;
    }

    public String getPassword() {
        return password;
    }

    public void setPassword(String password) {
        this.password = password;
    }

    public Set<String> getRoles() {
        return new HashSet<>(roles); // Retorna uma cópia defensiva
    }

    public void setRoles(Set<String> roles) {
        this.roles = roles != null ? new HashSet<>(roles) : new HashSet<>();
    }

    // Método para adicionar uma role
    public void addRole(String role) {
        this.roles.add(role);
    }

    // Método para remover uma role
    public void removeRole(String role) {
        this.roles.remove(role);
    }

    // Método toString para depuração
    @Override
    public String toString() {
        return "User{" +
                "id='" + id + '\'' +
                ", username='" + username + '\'' +
                ", roles=" + roles +
                '}';
    }

    // Override do método equals para comparação precisa
    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        User user = (User) o;
        return Objects.equals(id, user.id) &&
               Objects.equals(username, user.username) &&
               Objects.equals(roles, user.roles);
    }

    // Override do método hashCode para uso eficiente em coleções
    @Override
    public int hashCode() {
        return Objects.hash(id, username, roles);
    }
}

