package com.radael.challenge_api.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.challenge_api.model.Notification;

public interface NotificationRepository extends MongoRepository<Notification, String> {
    // Métodos de consulta personalizados, se necessário
}