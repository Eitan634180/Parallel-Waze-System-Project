import { clearMapLayers, clearRouteLayers, drawAlternatives, drawEndpointMarkers, drawRoute } from './layers/route-layers.js';
import { clearDebugCars, removeDebugCar, setDebugCarsVisible, syncDebugCars, upsertDebugCar } from './layers/debug-cars-layer.js';
import { refreshRouteInspector } from './layers/route-inspector.js';
import { initCarMarker, removeCarMarker, updateCarPositionAndRotation } from './layers/vehicle-layer.js';

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

    initMap(containerId = 'map') {
        this.map = L.map(containerId, {
            center: [32.0853, 34.7818],
            zoom: 12,
            zoomControl: true,
        });

        L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
            attribution: '&copy; <a href="https://carto.com/">CARTO</a>',
            subdomains: 'abcd',
            maxZoom: 19,
        }).addTo(this.map);
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
