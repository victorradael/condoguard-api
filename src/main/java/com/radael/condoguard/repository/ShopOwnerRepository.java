package com.radael.condoguard.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.condoguard.model.ShopOwner;

public interface ShopOwnerRepository extends MongoRepository<ShopOwner, String> {
    // Métodos de consulta personalizados, se necessário
}