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
