import { flipCoords } from '../../../utils/geo.js';
import { refreshRouteInspector } from './route-inspector.js';

export function drawRoute(manager, coordinates, source, dest) {
    clearLayer(manager, 'route');

    const latLngs = flipCoords(coordinates);
    manager.layers.route = L.polyline(latLngs, {
        color: '#33ccff',
        weight: 10,
        opacity: manager.routeInspector.enabled ? 0 : 0.9,
        lineCap: 'round',
        lineJoin: 'round',
    }).addTo(manager.map);

    manager.map.fitBounds(manager.layers.route.getBounds(), { padding: [100, 100] });
    drawWalkRoute(manager, coordinates, source, dest);
    refreshRouteInspector(manager);
}

export function drawAlternatives(manager, alternativeRoutes) {
    clearLayer(manager, 'altRoutes');

    const layers = alternativeRoutes.map((route) => L.polyline(flipCoords(route.pathCoords), {
        color: '#94a3b8',
        weight: 6,
        opacity: 0.6,
        dashArray: '10 10',
        lineCap: 'round',
    }));

    manager.layers.altRoutes = L.layerGroup(layers).addTo(manager.map);
}

export function drawEndpointMarkers(manager, source, dest) {
    clearLayer(manager, 'source');
    clearLayer(manager, 'dest');

    if (source) {
        manager.layers.source = L.circleMarker([source.lat, source.lng], {
            radius: 12,
            fillColor: '#33ccff',
            fillOpacity: 1,
            color: 'white',
            weight: 4,
        }).addTo(manager.map).bindPopup(`<b>Start:</b> ${source.name.split(',')[0]}`);
    }

    if (dest) {
        manager.layers.dest = L.circleMarker([dest.lat, dest.lng], {
            radius: 12,
            fillColor: '#ff3d00',
            fillOpacity: 1,
            color: 'white',
            weight: 4,
        }).addTo(manager.map).bindPopup(`<b>End:</b> ${dest.name.split(',')[0]}`);
    }
}

export function clearRouteLayers(manager) {
    ['route', 'altRoutes', 'walk', 'source', 'dest', 'debugRoute'].forEach((key) => clearLayer(manager, key));
    manager.routeInspector.routeObj = null;
    manager.routeInspector.edgeLines = [];
}

export function clearMapLayers(manager) {
    for (const marker of manager.layers.debugCars.values()) {
        manager.map.removeLayer(marker);
    }

    Object.entries(manager.layers).forEach(([key, layer]) => {
        if (layer && !(layer instanceof Map)) {
            manager.map.removeLayer(layer);
            if (key !== 'debugCars') {
                manager.layers[key] = null;
            }
        }
    });

    manager.layers = {
        route: null,
        altRoutes: null,
        walk: null,
        source: null,
        dest: null,
        car: null,
        debugRoute: null,
        debugCars: new Map(),
    };
    manager.routeInspector.routeObj = null;
    manager.routeInspector.edgeLines = [];
}

function drawWalkRoute(manager, coordinates, source, dest) {
    clearLayer(manager, 'walk');

    const walkLayers = [];
    if (source && coordinates.length > 0) {
        walkLayers.push(L.polyline([[source.lat, source.lng], [coordinates[0][1], coordinates[0][0]]], {
            color: '#33ccff',
            weight: 3,
            dashArray: '5, 8',
            opacity: 0.6,
        }));
    }

    if (dest && coordinates.length > 0) {
        const last = coordinates[coordinates.length - 1];
        walkLayers.push(L.polyline([[dest.lat, dest.lng], [last[1], last[0]]], {
            color: '#ff5252',
            weight: 3,
            dashArray: '5, 8',
            opacity: 0.6,
        }));
    }

    manager.layers.walk = L.layerGroup(walkLayers).addTo(manager.map);
}

function clearLayer(manager, key) {
    const layer = manager.layers[key];
    if (!layer) {
        return;
    }

    manager.map.removeLayer(layer);
    manager.layers[key] = null;
}
