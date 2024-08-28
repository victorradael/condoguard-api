package com.radael.challenge_api.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "residents")
public class Resident {
    @Id
    private String id;
    private String name;
    private String apartmentType; // "normal" ou "duplex"

    // Construtor padrão
    public Resident() {
    }

    // Construtor com parâmetros
    public Resident(String name, String apartmentType) {
        this.name = name;
        setApartmentType(apartmentType); // Utiliza o setter para validação
    }

    // Getters e Setters
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getApartmentType() {
        return apartmentType;
    }

    public void setApartmentType(String apartmentType) {
        if (!"normal".equalsIgnoreCase(apartmentType) && !"duplex".equalsIgnoreCase(apartmentType)) {
            throw new IllegalArgumentException("Tipo de apartamento inválido: " + apartmentType);
        }
        this.apartmentType = apartmentType;
    }

    // Método utilitário para verificar se o apartamento é duplex
    public boolean isDuplex() {
        return "duplex".equalsIgnoreCase(this.apartmentType);
    }

    // Método toString para facilitar a exibição dos dados do Resident
    @Override
    public String toString() {
        return "Resident{" +
                "id='" + id + '\'' +
                ", name='" + name + '\'' +
                ", apartmentType='" + apartmentType + '\'' +
                '}';
    }

    // Override do método equals para comparação precisa entre objetos Resident
    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;

        Resident resident = (Resident) o;

        if (!id.equals(resident.id)) return false;
        if (!name.equals(resident.name)) return false;
        return apartmentType.equals(resident.apartmentType);
    }

    // Override do método hashCode para uso eficiente em coleções
    @Override
    public int hashCode() {
        int result = id.hashCode();
        result = 31 * result + name.hashCode();
        result = 31 * result + apartmentType.hashCode();
        return result;
    }
}
