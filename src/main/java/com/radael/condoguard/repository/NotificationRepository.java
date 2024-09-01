package com.radael.condoguard.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.condoguard.model.Notification;

public interface NotificationRepository extends MongoRepository<Notification, String> {
    // Métodos de consulta personalizados, se necessário
}