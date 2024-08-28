package com.radael.challenge_api.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import com.radael.challenge_api.model.Notification;
import com.radael.challenge_api.service.NotificationService;

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
