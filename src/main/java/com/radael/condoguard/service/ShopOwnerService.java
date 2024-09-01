package com.radael.condoguard.service;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;

import com.radael.condoguard.model.ShopOwner;
import com.radael.condoguard.repository.ShopOwnerRepository;

import java.util.List;
import java.util.Optional;

@Service
public class ShopOwnerService {

    @Autowired
    private ShopOwnerRepository shopOwnerRepository;

    public List<ShopOwner> getAllShopOwners() {
        return shopOwnerRepository.findAll();
    }

    public Optional<ShopOwner> getShopOwnerById(String id) {
        return shopOwnerRepository.findById(id);
    }

    public ShopOwner createShopOwner(ShopOwner shopOwner) {
        return shopOwnerRepository.save(shopOwner);
    }

    public ShopOwner updateShopOwner(String id, ShopOwner shopOwnerDetails) {
        Optional<ShopOwner> optionalShopOwner = shopOwnerRepository.findById(id);
        if (optionalShopOwner.isPresent()) {
            ShopOwner shopOwner = optionalShopOwner.get();
            shopOwner.setName(shopOwnerDetails.getName());
            shopOwner.setShopName(shopOwnerDetails.getShopName());
            return shopOwnerRepository.save(shopOwner);
        }
        return null;
    }

    public void deleteShopOwner(String id) {
        shopOwnerRepository.deleteById(id);
    }
}
