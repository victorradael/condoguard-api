package com.radael.challenge_api.model;

import org.springframework.data.annotation.Id;
import org.springframework.data.mongodb.core.mapping.Document;

@Document(collection = "shopOwners")
public class ShopOwner {
    @Id
    private String id;
    private String name;
    private String shopName;

    // Construtor padrão
    public ShopOwner() {
    }

    // Construtor com parâmetros
    public ShopOwner(String name, String shopName) {
        this.name = name;
        this.shopName = shopName;
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

    public String getShopName() {
        return shopName;
    }

    public void setShopName(String shopName) {
        this.shopName = shopName;
    }

    // Método toString para facilitar a exibição dos dados do ShopOwner
    @Override
    public String toString() {
        return "ShopOwner{" +
                "id='" + id + '\'' +
                ", name='" + name + '\'' +
                ", shopName='" + shopName + '\'' +
                '}';
    }

    // Override do método equals para comparação precisa entre objetos ShopOwner
    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;

        ShopOwner shopOwner = (ShopOwner) o;

        if (!id.equals(shopOwner.id)) return false;
        if (!name.equals(shopOwner.name)) return false;
        return shopName.equals(shopOwner.shopName);
    }

    // Override do método hashCode para uso eficiente em coleções
    @Override
    public int hashCode() {
        int result = id.hashCode();
        result = 31 * result + name.hashCode();
        result = 31 * result + shopName.hashCode();
        return result;
    }
}
