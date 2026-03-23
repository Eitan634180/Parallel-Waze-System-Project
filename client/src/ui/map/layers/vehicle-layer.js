export function initCarMarker(manager, startPos) {
    const carIcon = L.divIcon({
        className: 'car-icon-container',
        html: '<div class="car-body"><img src="assets/car.svg" width="30" height="45"></div>',
        iconSize: [30, 45],
        iconAnchor: [15, 22],
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
