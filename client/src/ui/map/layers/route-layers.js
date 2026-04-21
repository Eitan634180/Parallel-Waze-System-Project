import { flipCoords } from '../../../utils/geo.js';
import { refreshRouteInspector } from './route-inspector.js';
import { MAP_COLORS, MAP_DEFAULTS, MAP_LAYER_STYLE } from '../map-config.js';

const MANAGED_ROUTE_LAYER_KEYS = ['route', 'altRoutes', 'walk', 'source', 'dest', 'debugRoute'];
const SOURCE_LABEL = 'Start:';
const DESTINATION_LABEL = 'End:';

export function drawRoute(manager, coordinates, source, dest) {
    clearLayer(manager, 'route');

    const latLngs = flipCoords(coordinates);
    manager.layers.route = L.polyline(latLngs, {
        color: MAP_COLORS.primaryRoute,
        weight: MAP_LAYER_STYLE.primaryRouteWeight,
        opacity: manager.routeInspector.enabled ? MAP_LAYER_STYLE.hiddenRouteOpacity : MAP_LAYER_STYLE.primaryRouteOpacity,
        lineCap: 'round',
        lineJoin: 'round',
    }).addTo(manager.map);

    manager.map.fitBounds(manager.layers.route.getBounds(), { padding: MAP_DEFAULTS.fitBoundsPadding });
    drawWalkRoute(manager, coordinates, source, dest);
    refreshRouteInspector(manager);
}

export function drawAlternatives(manager, alternativeRoutes) {
    clearLayer(manager, 'altRoutes');

    const layers = alternativeRoutes.map((route) => L.polyline(flipCoords(route.pathCoords), {
        color: MAP_COLORS.secondaryRoute,
        weight: MAP_LAYER_STYLE.secondaryRouteWeight,
        opacity: MAP_LAYER_STYLE.secondaryRouteOpacity,
        dashArray: MAP_LAYER_STYLE.secondaryRouteDashArray,
        lineCap: 'round',
    }));

    manager.layers.altRoutes = L.layerGroup(layers).addTo(manager.map);
}

export function drawEndpointMarkers(manager, source, dest) {
    clearLayer(manager, 'source');
    clearLayer(manager, 'dest');

    if (source) {
        manager.layers.source = L.circleMarker([source.lat, source.lng], {
            radius: MAP_LAYER_STYLE.endpointRadius,
            fillColor: MAP_COLORS.primaryRoute,
            fillOpacity: MAP_LAYER_STYLE.endpointFillOpacity,
            color: MAP_COLORS.endpointBorder,
            weight: MAP_LAYER_STYLE.endpointWeight,
        }).addTo(manager.map).bindPopup(`<b>${SOURCE_LABEL}</b> ${source.name.split(',')[0]}`);
    }

    if (dest) {
        manager.layers.dest = L.circleMarker([dest.lat, dest.lng], {
            radius: MAP_LAYER_STYLE.endpointRadius,
            fillColor: MAP_COLORS.destination,
            fillOpacity: MAP_LAYER_STYLE.endpointFillOpacity,
            color: MAP_COLORS.endpointBorder,
            weight: MAP_LAYER_STYLE.endpointWeight,
        }).addTo(manager.map).bindPopup(`<b>${DESTINATION_LABEL}</b> ${dest.name.split(',')[0]}`);
    }
}

export function clearRouteLayers(manager) {
    MANAGED_ROUTE_LAYER_KEYS.forEach((key) => clearLayer(manager, key));
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
            color: MAP_COLORS.primaryRoute,
            weight: MAP_LAYER_STYLE.walkRouteWeight,
            dashArray: MAP_LAYER_STYLE.walkRouteDashArray,
            opacity: MAP_LAYER_STYLE.walkRouteOpacity,
        }));
    }

    if (dest && coordinates.length > 0) {
        const last = coordinates[coordinates.length - 1];
        walkLayers.push(L.polyline([[dest.lat, dest.lng], [last[1], last[0]]], {
            color: MAP_COLORS.destinationWalk,
            weight: MAP_LAYER_STYLE.walkRouteWeight,
            dashArray: MAP_LAYER_STYLE.walkRouteDashArray,
            opacity: MAP_LAYER_STYLE.walkRouteOpacity,
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
