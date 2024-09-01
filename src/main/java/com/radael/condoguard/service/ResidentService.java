package com.radael.condoguard.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import com.radael.condoguard.model.Resident;
import com.radael.condoguard.repository.ResidentRepository;

import java.util.List;
import java.util.Optional;

@Service
public class ResidentService {

    @Autowired
    private ResidentRepository residentRepository;

    public List<Resident> getAllResidents() {
        return residentRepository.findAll();
    }

    public Optional<Resident> getResidentById(String id) {
        return residentRepository.findById(id);
    }

    public Resident createResident(Resident resident) {
        return residentRepository.save(resident);
    }

    public Resident updateResident(String id, Resident residentDetails) {
        Optional<Resident> optionalResident = residentRepository.findById(id);
        if (optionalResident.isPresent()) {
            Resident resident = optionalResident.get();
            resident.setName(residentDetails.getName());
            resident.setApartmentType(residentDetails.getApartmentType());
            return residentRepository.save(resident);
        }
        return null;
    }

    public void deleteResident(String id) {
        residentRepository.deleteById(id);
    }
}

