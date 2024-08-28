package com.radael.challenge_api.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.challenge_api.model.ShopOwner;

public interface ShopOwnerRepository extends MongoRepository<ShopOwner, String> {
    // Métodos de consulta personalizados, se necessário
}