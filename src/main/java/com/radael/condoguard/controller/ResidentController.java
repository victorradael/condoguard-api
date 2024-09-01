package com.radael.condoguard.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import com.radael.condoguard.model.Resident;
import com.radael.condoguard.service.ResidentService;

import java.util.List;
import java.util.Optional;

@RestController
@RequestMapping("/residents")
public class ResidentController {

    @Autowired
    private ResidentService residentService;

    @GetMapping
    public List<Resident> getAllResidents() {
        return residentService.getAllResidents();
    }

    @GetMapping("/{id}")
    public Optional<Resident> getResidentById(@PathVariable String id) {
        return residentService.getResidentById(id);
    }

    @PostMapping
    public Resident createResident(@RequestBody Resident resident) {
        return residentService.createResident(resident);
    }

    @PutMapping("/{id}")
    public Resident updateResident(@PathVariable String id, @RequestBody Resident residentDetails) {
        return residentService.updateResident(id, residentDetails);
    }

    @DeleteMapping("/{id}")
    public void deleteResident(@PathVariable String id) {
        residentService.deleteResident(id);
    }
}
