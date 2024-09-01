package com.radael.condoguard.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.condoguard.model.Resident;

public interface ResidentRepository extends MongoRepository<Resident, String> {
    // Métodos de consulta personalizados, se necessário
}