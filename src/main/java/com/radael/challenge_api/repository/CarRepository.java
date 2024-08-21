package com.radael.challenge_api.repository;

import com.radael.challenge_api.model.Car;
import org.springframework.data.mongodb.repository.MongoRepository;
import org.springframework.stereotype.Repository;

@Repository
public interface CarRepository extends MongoRepository<Car, String> {
    // Adicione métodos personalizados, se necessário
}
