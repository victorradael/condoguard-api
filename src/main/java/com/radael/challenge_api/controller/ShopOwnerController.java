package com.radael.challenge_api.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.web.bind.annotation.*;

import com.radael.challenge_api.model.ShopOwner;
import com.radael.challenge_api.service.ShopOwnerService;

import java.util.List;
import java.util.Optional;

@RestController
@RequestMapping("/shopowners")
public class ShopOwnerController {

    @Autowired
    private ShopOwnerService shopOwnerService;

    @GetMapping
    public List<ShopOwner> getAllShopOwners() {
        return shopOwnerService.getAllShopOwners();
    }

    @GetMapping("/{id}")
    public Optional<ShopOwner> getShopOwnerById(@PathVariable String id) {
        return shopOwnerService.getShopOwnerById(id);
    }

    @PostMapping
    public ShopOwner createShopOwner(@RequestBody ShopOwner shopOwner) {
        return shopOwnerService.createShopOwner(shopOwner);
    }

    @PutMapping("/{id}")
    public ShopOwner updateShopOwner(@PathVariable String id, @RequestBody ShopOwner shopOwnerDetails) {
        return shopOwnerService.updateShopOwner(id, shopOwnerDetails);
    }

    @DeleteMapping("/{id}")
    public void deleteShopOwner(@PathVariable String id) {
        shopOwnerService.deleteShopOwner(id);
    }
}
