package com.radael.condoguard.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import com.radael.condoguard.model.Notification;
import com.radael.condoguard.service.NotificationService;

@RestController
@RequestMapping("/notifications")
public class NotificationController {

    private final NotificationService notificationService;

    @Autowired
    public NotificationController(NotificationService notificationService) {
        this.notificationService = notificationService;
    }

    @PostMapping("/send")
    public void sendNotification(@RequestBody Notification request) {
        notificationService.sendNotificationToGroup(request.getGroupName(), request.getContent());
    }
}
