package com.radael.condoguard.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.condoguard.model.User;

public interface UserRepository extends MongoRepository<User, String> {
    User findByUsername(String username);
}

