package com.radael.challenge_api.service;

import com.radael.challenge_api.model.Car;
import com.radael.challenge_api.repository.CarRepository;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
public class CarService {
    
    @Autowired
    private CarRepository carRepository;
    
    public List<Car> getAllCars() {
        return carRepository.findAll();
    }

    public Car addCar(Car car) {
        return carRepository.save(car);
    }
}
