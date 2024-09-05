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

import com.radael.condoguard.model.Notification;
import com.radael.condoguard.repository.NotificationRepository;
import com.radael.condoguard.repository.ResidentRepository;
import com.radael.condoguard.repository.ShopOwnerRepository;

import java.util.List;
import java.util.Optional;

@Service
public class NotificationService {

    @Autowired
    private NotificationRepository notificationRepository;

    @Autowired
    private ResidentRepository residentRepository;

    @Autowired
    private ShopOwnerRepository shopOwnerRepository;

    public List<Notification> getAllNotifications() {
        return notificationRepository.findAll();
    }

    public Optional<Notification> getNotificationById(String id) {
        return notificationRepository.findById(id);
    }

    public Notification createNotification(Notification notification) {
        // Verifica se os residentes associados existem
        if (notification.getResidents() != null) {
            notification.setResidents(residentRepository.findAllById(notification.getResidents().stream().map(resident -> resident.getId()).toList()));
        }

        // Verifica se os proprietários de loja associados existem
        if (notification.getShopOwners() != null) {
            notification.setShopOwners(shopOwnerRepository.findAllById(notification.getShopOwners().stream().map(shopOwner -> shopOwner.getId()).toList()));
        }

        return notificationRepository.save(notification);
    }

    public Notification updateNotification(String id, Notification notificationDetails) {
        Notification notification = notificationRepository.findById(id).orElseThrow(() -> new RuntimeException("Notification not found"));
        notification.setMessage(notificationDetails.getMessage());
        notification.setCreatedBy(notificationDetails.getCreatedBy());
        
        // Atualiza a lista de residentes associados
        if (notificationDetails.getResidents() != null) {
            notification.setResidents(residentRepository.findAllById(notificationDetails.getResidents().stream().map(resident -> resident.getId()).toList()));
        }

        // Atualiza a lista de proprietários de loja associados
        if (notificationDetails.getShopOwners() != null) {
            notification.setShopOwners(shopOwnerRepository.findAllById(notificationDetails.getShopOwners().stream().map(shopOwner -> shopOwner.getId()).toList()));
        }

        return notificationRepository.save(notification);
    }

    public void deleteNotification(String id) {
        notificationRepository.deleteById(id);
    }
}
