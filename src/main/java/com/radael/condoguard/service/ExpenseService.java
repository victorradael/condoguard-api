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

package com.radael.condoguard.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import com.radael.condoguard.model.Expense;
import com.radael.condoguard.repository.ExpenseRepository;
import com.radael.condoguard.repository.ResidentRepository;
import com.radael.condoguard.repository.ShopOwnerRepository;

import java.util.List;
import java.util.Optional;

@Service
public class ExpenseService {

    @Autowired
    private ExpenseRepository expenseRepository;

    @Autowired
    private ResidentRepository residentRepository;

    @Autowired
    private ShopOwnerRepository shopOwnerRepository;

    public List<Expense> getAllExpenses() {
        return expenseRepository.findAll();
    }

    public Optional<Expense> getExpenseById(String id) {
        return expenseRepository.findById(id);
    }

    public Expense createExpense(Expense expense) {
        // Verifica se o residente associado existe
        if (expense.getResident() != null) {
            residentRepository.findById(expense.getResident().getId())
                    .orElseThrow(() -> new RuntimeException("Resident not found"));
        }

        // Verifica se o proprietário de loja associado existe
        if (expense.getShopOwner() != null) {
            shopOwnerRepository.findById(expense.getShopOwner().getId())
                    .orElseThrow(() -> new RuntimeException("ShopOwner not found"));
        }

        return expenseRepository.save(expense);
    }

    public Expense updateExpense(String id, Expense expenseDetails) {
        Expense expense = expenseRepository.findById(id).orElseThrow(() -> new RuntimeException("Expense not found"));

        expense.setDescription(expenseDetails.getDescription());
        expense.setAmount(expenseDetails.getAmount());
        expense.setDate(expenseDetails.getDate());

        // Atualiza a referência para o residente, se aplicável
        if (expenseDetails.getResident() != null) {
            residentRepository.findById(expenseDetails.getResident().getId())
                    .orElseThrow(() -> new RuntimeException("Resident not found"));
            expense.setResident(expenseDetails.getResident());
        }

        // Atualiza a referência para o proprietário de loja, se aplicável
        if (expenseDetails.getShopOwner() != null) {
            shopOwnerRepository.findById(expenseDetails.getShopOwner().getId())
                    .orElseThrow(() -> new RuntimeException("ShopOwner not found"));
            expense.setShopOwner(expenseDetails.getShopOwner());
        }

        return expenseRepository.save(expense);
    }

    public void deleteExpense(String id) {
        expenseRepository.deleteById(id);
    }
}

