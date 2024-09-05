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

import java.util.Date;
import java.util.List;
import java.util.Objects;

@Document(collection = "notifications")
public class Notification {
    @Id
    private String id;
    private String message; // Mensagem da notificação
    private User createdBy; // Usuário que criou a notificação
    private Date createdAt; // Data de criação da notificação
    private List<Resident> residents; // Destinatários (residências)
    private List<ShopOwner> shopOwners; // Destinatários (lojas)

    // Construtor padrão
    public Notification() {}

    // Construtor com parâmetros
    public Notification(String message, User createdBy, Date createdAt, List<Resident> residents, List<ShopOwner> shopOwners) {
        this.message = message;
        this.createdBy = createdBy;
        this.createdAt = (createdAt != null) ? new Date(createdAt.getTime()) : null; // Cópia defensiva para evitar mutabilidade
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

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }

    public User getCreatedBy() {
        return createdBy;
    }

    public void setCreatedBy(User createdBy) {
        this.createdBy = createdBy;
    }

    public Date getCreatedAt() {
        return (createdAt != null) ? new Date(createdAt.getTime()) : null; // Retorna uma cópia defensiva
    }

    public void setCreatedAt(Date createdAt) {
        this.createdAt = (createdAt != null) ? new Date(createdAt.getTime()) : null;
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
        return "Notification{" +
                "id='" + id + '\'' +
                ", message='" + message + '\'' +
                ", createdBy=" + (createdBy != null ? createdBy.getUsername() : null) +
                ", createdAt=" + createdAt +
                ", residents=" + residents +
                ", shopOwners=" + shopOwners +
                '}';
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        Notification that = (Notification) o;
        return Objects.equals(id, that.id) &&
               Objects.equals(message, that.message) &&
               Objects.equals(createdBy, that.createdBy) &&
               Objects.equals(createdAt, that.createdAt) &&
               Objects.equals(residents, that.residents) &&
               Objects.equals(shopOwners, that.shopOwners);
    }

    @Override
    public int hashCode() {
        return Objects.hash(id, message, createdBy, createdAt, residents, shopOwners);
    }
}
