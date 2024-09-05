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

import java.util.List;
import java.util.Objects;

@Document(collection = "shopOwners")
public class ShopOwner {
    @Id
    private String id;
    private String shopName; // Nome da loja
    private int floor; // Andar onde a loja está localizada
    private User owner; // Associação com o proprietário (usuário)
    private List<Expense> expenses; // Lista de despesas associadas
    private List<Notification> notifications; // Lista de notificações associadas

    // Construtor padrão
    public ShopOwner() {}

    // Construtor com parâmetros
    public ShopOwner(String shopName, int floor, User owner, List<Expense> expenses, List<Notification> notifications) {
        this.shopName = shopName;
        this.floor = floor;
        this.owner = owner;
        this.expenses = expenses;
        this.notifications = notifications;
    }

    // Getters e Setters
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getShopName() {
        return shopName;
    }

    public void setShopName(String shopName) {
        this.shopName = shopName;
    }

    public int getFloor() {
        return floor;
    }

    public void setFloor(int floor) {
        this.floor = floor;
    }

    public User getOwner() {
        return owner;
    }

    public void setOwner(User owner) {
        this.owner = owner;
    }

    public List<Expense> getExpenses() {
        return expenses;
    }

    public void setExpenses(List<Expense> expenses) {
        this.expenses = expenses;
    }

    public List<Notification> getNotifications() {
        return notifications;
    }

    public void setNotifications(List<Notification> notifications) {
        this.notifications = notifications;
    }

    // Métodos toString, equals e hashCode
    @Override
    public String toString() {
        return "ShopOwner{" +
                "id='" + id + '\'' +
                ", shopName='" + shopName + '\'' +
                ", floor=" + floor +
                ", owner=" + (owner != null ? owner.getUsername() : null) +
                ", expenses=" + expenses +
                ", notifications=" + notifications +
                '}';
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        ShopOwner shopOwner = (ShopOwner) o;
        return floor == shopOwner.floor &&
               Objects.equals(id, shopOwner.id) &&
               Objects.equals(shopName, shopOwner.shopName) &&
               Objects.equals(owner, shopOwner.owner) &&
               Objects.equals(expenses, shopOwner.expenses) &&
               Objects.equals(notifications, shopOwner.notifications);
    }

    @Override
    public int hashCode() {
        return Objects.hash(id, shopName, floor, owner, expenses, notifications);
    }
}

