/*
 * This file is part of CondoGuard.
 *
 * CondoGuard is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * CondoGuard is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with CondoGuard. If not, see <https://www.gnu.org/licenses/>.
 */

package com.radael.condoguard.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

import java.util.HashSet;
import java.util.List;
import java.util.Objects;
import java.util.Set;

@Document(collection = "users")
public class User {
    @Id
    private String id;
    private String username;
    private String password; // Armazenar hash da senha
    private String email;
    private Set<String> roles = new HashSet<>(); // Ex: ["ROLE_USER", "ROLE_ADMIN"]
    private List<Resident> residents; // Associação com residências
    private List<ShopOwner> shopOwners; // Associação com lojas

    // Construtor padrão
    public User() {}

    // Construtor com parâmetros
    public User(String username, String password, String email, Set<String> roles,
                List<Resident> residents, List<ShopOwner> shopOwners) {
        this.username = username;
        this.setPassword(password); // Usa o setter para garantir o hash
        this.email = email;
        this.roles = roles != null ? new HashSet<>(roles) : new HashSet<>();
        this.residents = residents;
        this.shopOwners = shopOwners;
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

    public String getEmail() {
        return email;
    }

    public void setEmail(String email) {
        this.email = email;
    }

    public Set<String> getRoles() {
        return new HashSet<>(roles); // Retorna uma cópia defensiva
    }

    public void setRoles(Set<String> roles) {
        this.roles = roles != null ? new HashSet<>(roles) : new HashSet<>();
    }

    public List<Resident> getResidents() {
        return residents;
    }

    public void setResidents(List<Resident> residents) {
        this.residents = residents;
    }

    public List<ShopOwner> getShopOwners() {
        return shopOwners;
    }

    public void setShopOwners(List<ShopOwner> shopOwners) {
        this.shopOwners = shopOwners;
    }

    // Métodos toString, equals e hashCode
    @Override
    public String toString() {
        return "User{" +
                "id='" + id + '\'' +
                ", username='" + username + '\'' +
                ", email='" + email + '\'' +
                ", roles=" + roles +
                ", residents=" + residents +
                ", shopOwners=" + shopOwners +
                '}';
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        User user = (User) o;
        return Objects.equals(id, user.id) &&
               Objects.equals(username, user.username) &&
               Objects.equals(email, user.email) &&
               Objects.equals(roles, user.roles) &&
               Objects.equals(residents, user.residents) &&
               Objects.equals(shopOwners, user.shopOwners);
    }

    @Override
    public int hashCode() {
        return Objects.hash(id, username, email, roles, residents, shopOwners);
    }
}
