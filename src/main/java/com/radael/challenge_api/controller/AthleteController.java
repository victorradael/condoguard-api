package com.radael.challenge_api.controller;

import java.util.List;
import java.util.stream.Collectors;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

import com.radael.challenge_api.dto.AthleteDTO;
import com.radael.challenge_api.models.Athlete;




@RestController
public class AthleteController {

    
    
    @GetMapping("/default")
    public List<AthleteDTO> Default(){

        Athlete repository = new Athlete();
        
        return repository.getAll().stream().map(s -> new AthleteDTO(s.getName(), s.getEmail(), s.getBrithday())).collect(Collectors.toList());
    }
}
