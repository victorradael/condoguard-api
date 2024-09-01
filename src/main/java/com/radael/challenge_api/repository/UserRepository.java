package com.radael.challenge_api.repository;

import org.springframework.data.mongodb.repository.MongoRepository;

import com.radael.challenge_api.model.User;

public interface UserRepository extends MongoRepository<User, String> {
    User findByUsername(String username);
}

