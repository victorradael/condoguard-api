/*
 * This file is part of CondoGuard.
 *
 * CondoGuard is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * CondoGuard is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with CondoGuard. If not, see <https://www.gnu.org/licenses/>.
 */

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
        Resident resident = residentRepository.findById(id).orElseThrow(() -> new RuntimeException("Resident not found"));
        resident.setUnitNumber(residentDetails.getUnitNumber());
        resident.setFloor(residentDetails.getFloor());
        // Atualize outros campos conforme necessário
        return residentRepository.save(resident);
    }

    public void deleteResident(String id) {
        residentRepository.deleteById(id);
    }
}


