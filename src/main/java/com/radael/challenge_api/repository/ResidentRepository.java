package com.radael.challenge_api.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.challenge_api.model.Resident;

public interface ResidentRepository extends MongoRepository<Resident, String> {
    // Métodos de consulta personalizados, se necessário
}