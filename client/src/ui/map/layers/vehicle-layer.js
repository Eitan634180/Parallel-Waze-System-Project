import { MAP_LAYER_STYLE } from '../map-config.js';

const VEHICLE_ICON_HTML = `<div class="car-body"><img src="assets/car.svg" width="${MAP_LAYER_STYLE.vehicleIconSize[0]}" height="${MAP_LAYER_STYLE.vehicleIconSize[1]}"></div>`;

export function initCarMarker(manager, startPos) {
    const carIcon = L.divIcon({
        className: 'car-icon-container',
        html: VEHICLE_ICON_HTML,
        iconSize: MAP_LAYER_STYLE.vehicleIconSize,
        iconAnchor: MAP_LAYER_STYLE.vehicleIconAnchor,
    });

    removeCarMarker(manager);
    manager.layers.car = L.marker(startPos, {
        icon: carIcon,
        zoomPanOptions: { animate: true },
    }).addTo(manager.map);
}

export function updateCarPositionAndRotation(manager, pos, bearing) {
    if (!manager.layers.car) {
        return;
    }

    manager.layers.car.setLatLng(pos);
    const carBody = manager.layers.car.getElement().querySelector('.car-body');
    if (carBody) {
        carBody.style.transform = `rotate(${bearing}deg)`;
    }
}

export function removeCarMarker(manager) {
    if (!manager.layers.car) {
        return;
    }

    manager.map.removeLayer(manager.layers.car);
    manager.layers.car = null;
}
