package com.radael.challenge_api.models;

import java.util.ArrayList;
import java.util.List;

public class Athlete {
    private String name;
    private String email;
    private String birthday;

    // Construtor
    public Athlete() {

    }

    // Métodos getters
    public String getName() {
        return name;
    }

    public String getEmail() {
        return email;
    }

    public String getBrithday() {
        return birthday;
    }

    public List<Athlete> getAll() {
        Athlete athlete = new Athlete();
        athlete.name = "Victor";
        athlete.email = "victor@email.com";
        athlete.birthday = "1999-12-31";

        List<Athlete> athletes = new ArrayList<>();
        athletes.add(athlete);
        athletes.add(athlete);
        athletes.add(athlete);

        return athletes;
    }

    // Método para exibir as informações do athletero
    @Override
    public String toString() {
        return "Athlete{" +
                "name='" + name + '\'' +
                ", email='" + email + '\'' +
                ", birthday=" + birthday +
                '}';
    }
}

