import { clearMapLayers, clearRouteLayers, drawAlternatives, drawEndpointMarkers, drawRoute } from './layers/route-layers.js';
import { clearDebugCars, removeDebugCar, setDebugCarsVisible, syncDebugCars, upsertDebugCar } from './layers/debug-cars-layer.js';
import { refreshRouteInspector } from './layers/route-inspector.js';
import { initCarMarker, removeCarMarker, updateCarPositionAndRotation } from './layers/vehicle-layer.js';
import { MAP_DEFAULTS, MAP_TILES } from './map-config.js';

function hasValidBoundingBox(systemInfo) {
    if (!systemInfo?.bbox) return false;

    const { min_lat, max_lat, min_lon, max_lon } = systemInfo.bbox;
    return Number.isFinite(min_lat)
        && Number.isFinite(max_lat)
        && Number.isFinite(min_lon)
        && Number.isFinite(max_lon)
        && max_lat > min_lat
        && max_lon > min_lon;
}

class MapManager {
    constructor() {
        this.map = null;
        this.layers = {
            route: null,
            altRoutes: null,
            walk: null,
            source: null,
            dest: null,
            car: null,
            debugRoute: null,
            debugCars: new Map(),
        };
        this.routeInspector = {
            enabled: false,
            routeObj: null,
            onEdgeClick: null,
            edgeLines: [],
        };
        this.debugCarsVisible = true;
    }

    initMap(containerId = 'map', systemInfo = null) {
        this.map = L.map(containerId, {
            center: MAP_DEFAULTS.center,
            zoom: MAP_DEFAULTS.zoom,
            zoomControl: true,
        });

        L.tileLayer(MAP_TILES.url, {
            attribution: MAP_TILES.attribution,
            subdomains: MAP_TILES.subdomains,
            maxZoom: MAP_DEFAULTS.maxZoom,
        }).addTo(this.map);

        if (hasValidBoundingBox(systemInfo)) {
            const { min_lat, max_lat, min_lon, max_lon } = systemInfo.bbox;
            this.map.fitBounds(
                [[min_lat, min_lon], [max_lat, max_lon]],
                { padding: MAP_DEFAULTS.fitBoundsPadding },
            );
        } else if (Number.isFinite(systemInfo?.center_lat) && Number.isFinite(systemInfo?.center_lon)) {
            this.map.setView([systemInfo.center_lat, systemInfo.center_lon], MAP_DEFAULTS.zoom);
        }
    }

    drawRoute(coordinates, source, dest) {
        drawRoute(this, coordinates, source, dest);
    }

    drawAlternatives(altRoutes) {
        drawAlternatives(this, altRoutes);
    }

    drawEndpointMarkers(source, dest) {
        drawEndpointMarkers(this, source, dest);
    }

    setRouteInspectorEnabled(enabled, onEdgeClick = null) {
        this.routeInspector.enabled = enabled;
        this.routeInspector.onEdgeClick = onEdgeClick;
        if (this.layers.route) {
            this.layers.route.setStyle({ opacity: enabled ? 0 : 0.9 });
        }
        this.refreshRouteInspector();
    }

    setRouteInspectorRoute(routeObj) {
        this.routeInspector.routeObj = routeObj;
        refreshRouteInspector(this);
    }

    refreshRouteInspector() {
        refreshRouteInspector(this);
    }

    clearMapLayers() {
        clearMapLayers(this);
    }

    clearRouteLayers() {
        clearRouteLayers(this);
    }

    initCarMarker(startPos) {
        initCarMarker(this, startPos);
    }

    updateCarPositionAndRotation(pos, bearing) {
        updateCarPositionAndRotation(this, pos, bearing);
    }

    removeCarMarker() {
        removeCarMarker(this);
    }

    upsertDebugCar(id, pos) {
        upsertDebugCar(this, id, pos);
    }

    removeDebugCar(id) {
        removeDebugCar(this, id);
    }

    clearDebugCars() {
        clearDebugCars(this);
    }

    syncDebugCars(cars) {
        syncDebugCars(this, cars);
    }

    setDebugCarsVisible(visible) {
        setDebugCarsVisible(this, visible);
    }
}

export const mapInstance = new MapManager();
