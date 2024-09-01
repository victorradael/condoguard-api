package com.radael.condoguard.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import com.radael.condoguard.model.Notification;
import com.radael.condoguard.model.Resident;
import com.radael.condoguard.model.ShopOwner;
import com.radael.condoguard.repository.NotificationRepository;
import com.radael.condoguard.repository.ResidentRepository;
import com.radael.condoguard.repository.ShopOwnerRepository;

import java.util.Date;
import java.util.List;

@Service
public class NotificationService {

    @Autowired
    private ResidentRepository residentRepository;

    @Autowired
    private ShopOwnerRepository shopOwnerRepository;

    @Autowired
    private NotificationRepository notificationRepository;

    public void sendNotificationToGroup(String groupName, String content) {
        Notification notification = new Notification();
        notification.setContent(content);
        notification.setGroupName(groupName);
        notification.setSentAt(new Date());
        notificationRepository.save(notification);

        if (groupName.equals("Apartamentos Normais")) {
            List<Resident> residents = residentRepository.findAll();
            residents.stream()
                     .filter(resident -> resident.getApartmentType().equals("normal"))
                     .forEach(resident -> System.out.println("Notificando: " + resident.getName()));
        } else if (groupName.equals("Apartamentos Duplex")) {
            List<Resident> residents = residentRepository.findAll();
            residents.stream()
                     .filter(resident -> resident.getApartmentType().equals("duplex"))
                     .forEach(resident -> System.out.println("Notificando: " + resident.getName()));
        } else if (groupName.equals("Lojas")) {
            List<ShopOwner> shopOwners = shopOwnerRepository.findAll();
            shopOwners.forEach(shopOwner -> System.out.println("Notificando: " + shopOwner.getName()));
        }
    }
}
