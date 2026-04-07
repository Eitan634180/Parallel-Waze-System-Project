import { MAP_DEFAULTS, MAP_LAYER_STYLE } from '../map-config.js';

const DEBUG_CAR_ICON_HTML = '<div class="debug-car-body"></div>';

export function upsertDebugCar(manager, id, pos) {
    if (!pos) {
        return;
    }

    let marker = manager.layers.debugCars.get(id);
    if (!marker) {
        const debugCarIcon = L.divIcon({
            className: 'debug-car-icon-container',
            html: DEBUG_CAR_ICON_HTML,
            iconSize: MAP_LAYER_STYLE.debugCarIconSize,
            iconAnchor: MAP_LAYER_STYLE.debugCarIconAnchor,
        });
        marker = L.marker(pos, { icon: debugCarIcon, interactive: false, zIndexOffset: MAP_DEFAULTS.debugCarZIndexOffset });
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
    const bounds = manager.map?.getBounds?.() || null;
    const liveIds = new Set();
    cars.forEach((car) => {
        if (!car?.id) {
            return;
        }

        if (bounds && !bounds.contains([car.lat, car.lon])) {
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
