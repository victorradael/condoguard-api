package com.radael.challenge_api.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "carsdb")
public class Car {
    @Id
    private String id;
    private String make;
    private String model;
    private int year;
    private String color;
    private double price;

    // Getter e Setter para id
    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    // Getter e Setter para make
    public String getMake() {
        return make;
    }

    public void setMake(String make) {
        this.make = make;
    }

    // Getter e Setter para model
    public String getModel() {
        return model;
    }

    public void setModel(String model) {
        this.model = model;
    }

    // Getter e Setter para year
    public int getYear() {
        return year;
    }

    public void setYear(int year) {
        this.year = year;
    }

    // Getter e Setter para color
    public String getColor() {
        return color;
    }

    public void setColor(String color) {
        this.color = color;
    }

    // Getter e Setter para price
    public double getPrice() {
        return price;
    }

    public void setPrice(double price) {
        this.price = price;
    }
}
