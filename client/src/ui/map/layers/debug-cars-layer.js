export function upsertDebugCar(manager, id, pos) {
    if (!pos) {
        return;
    }

    let marker = manager.layers.debugCars.get(id);
    if (!marker) {
        const debugCarIcon = L.divIcon({
            className: 'debug-car-icon-container',
            html: '<div class="debug-car-body"></div>',
            iconSize: [14, 14],
            iconAnchor: [7, 7],
        });
        marker = L.marker(pos, { icon: debugCarIcon, interactive: false, zIndexOffset: 600 });
        if (manager.debugCarsVisible) {
            marker.addTo(manager.map);
        }
        manager.layers.debugCars.set(id, marker);
        return;
    }

    marker.setLatLng(pos);
    if (manager.debugCarsVisible && !manager.map.hasLayer(marker)) {
        marker.addTo(manager.map);
    }
}

export function removeDebugCar(manager, id) {
    const marker = manager.layers.debugCars.get(id);
    if (!marker) {
        return;
    }

    manager.map.removeLayer(marker);
    manager.layers.debugCars.delete(id);
}

export function clearDebugCars(manager) {
    for (const marker of manager.layers.debugCars.values()) {
        manager.map.removeLayer(marker);
    }
    manager.layers.debugCars.clear();
}

export function syncDebugCars(manager, cars) {
    const liveIds = new Set();
    cars.forEach((car) => {
        if (!car?.id) {
            return;
        }

        liveIds.add(car.id);
        upsertDebugCar(manager, car.id, [car.lat, car.lon]);
    });

    for (const [id] of manager.layers.debugCars.entries()) {
        if (!liveIds.has(id)) {
            removeDebugCar(manager, id);
        }
    }
}

export function setDebugCarsVisible(manager, visible) {
    manager.debugCarsVisible = visible;
    for (const marker of manager.layers.debugCars.values()) {
        const isOnMap = manager.map.hasLayer(marker);
        if (visible && !isOnMap) {
            marker.addTo(manager.map);
        } else if (!visible && isOnMap) {
            manager.map.removeLayer(marker);
        }
    }
}
