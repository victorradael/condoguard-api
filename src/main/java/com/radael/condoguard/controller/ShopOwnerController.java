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

package com.radael.condoguard.controller;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import com.radael.condoguard.model.ShopOwner;
import com.radael.condoguard.service.ShopOwnerService;

import java.util.List;

@RestController
@RequestMapping("/shopOwners")
public class ShopOwnerController {
    
    @Autowired
    private ShopOwnerService shopOwnerService;

    @GetMapping
    public List<ShopOwner> getAllShopOwners() {
        return shopOwnerService.getAllShopOwners();
    }

    @GetMapping("/{id}")
    public ResponseEntity<ShopOwner> getShopOwnerById(@PathVariable String id) {
        return shopOwnerService.getShopOwnerById(id)
                .map(ResponseEntity::ok)
                .orElse(ResponseEntity.notFound().build());
    }

    @PostMapping
    public ShopOwner createShopOwner(@RequestBody ShopOwner shopOwner) {
        return shopOwnerService.createShopOwner(shopOwner);
    }

    @PutMapping("/{id}")
    public ResponseEntity<ShopOwner> updateShopOwner(@PathVariable String id, @RequestBody ShopOwner shopOwnerDetails) {
        return ResponseEntity.ok(shopOwnerService.updateShopOwner(id, shopOwnerDetails));
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> deleteShopOwner(@PathVariable String id) {
        shopOwnerService.deleteShopOwner(id);
        return ResponseEntity.noContent().build();
    }
}
