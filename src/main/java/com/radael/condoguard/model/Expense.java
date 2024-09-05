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
import java.util.Objects;

@Document(collection = "expenses")
public class Expense {
    @Id
    private String id;
    private String description;
    private double amount;
    private Date date;
    private Resident resident; // Associação opcional com uma residência
    private ShopOwner shopOwner; // Associação opcional com uma loja

    // Construtor padrão
    public Expense() {}

    // Construtor com parâmetros
    public Expense(String description, double amount, Date date, Resident resident, ShopOwner shopOwner) {
        this.description = description;
        this.amount = amount;
        this.date = (date != null) ? new Date(date.getTime()) : null; // Cópia defensiva para evitar mutabilidade
        this.resident = resident;
        this.shopOwner = shopOwner;
    }

    // Getters e Setters
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public double getAmount() {
        return amount;
    }

    public void setAmount(double amount) {
        this.amount = amount;
    }

    public Date getDate() {
        return (date != null) ? new Date(date.getTime()) : null; // Retorna uma cópia defensiva
    }

    public void setDate(Date date) {
        this.date = (date != null) ? new Date(date.getTime()) : null;
    }

    public Resident getResident() {
        return resident;
    }

    public void setResident(Resident resident) {
        this.resident = resident;
    }

    public ShopOwner getShopOwner() {
        return shopOwner;
    }

    public void setShopOwner(ShopOwner shopOwner) {
        this.shopOwner = shopOwner;
    }

    // Métodos toString, equals e hashCode
    @Override
    public String toString() {
        return "Expense{" +
                "id='" + id + '\'' +
                ", description='" + description + '\'' +
                ", amount=" + amount +
                ", date=" + date +
                ", resident=" + (resident != null ? resident.getId() : null) +
                ", shopOwner=" + (shopOwner != null ? shopOwner.getId() : null) +
                '}';
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        Expense expense = (Expense) o;
        return Double.compare(expense.amount, amount) == 0 &&
               Objects.equals(id, expense.id) &&
               Objects.equals(description, expense.description) &&
               Objects.equals(date, expense.date) &&
               Objects.equals(resident, expense.resident) &&
               Objects.equals(shopOwner, expense.shopOwner);
    }

    @Override
    public int hashCode() {
        return Objects.hash(id, description, amount, date, resident, shopOwner);
    }
}
