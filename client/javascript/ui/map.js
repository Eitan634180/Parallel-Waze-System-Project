import { flipCoords } from '../utils/utils.js';

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
        if (this.layers.route) this.map.removeLayer(this.layers.route);

        const latLngs = flipCoords(coordinates);
        this.layers.route = L.polyline(latLngs, {
            color: '#33ccff',
            weight: 10,
            opacity: this.routeInspector.enabled ? 0 : 0.9,
            lineCap: 'round',
            lineJoin: 'round',
        }).addTo(this.map);

        this.map.fitBounds(this.layers.route.getBounds(), { padding: [100, 100] });
        this.drawWalkRoute(coordinates, source, dest);
        this.refreshRouteInspector();
    }

    drawAlternatives(altRoutes) {
        if (this.layers.altRoutes) this.map.removeLayer(this.layers.altRoutes);

        const layerArray = altRoutes.map(routeObj => {
            const latLngs = flipCoords(routeObj.pathCoords);
            return L.polyline(latLngs, {
                color: '#94a3b8',
                weight: 6,
                opacity: 0.6,
                dashArray: '10 10',
                lineCap: 'round',
            });
        });

        this.layers.altRoutes = L.layerGroup(layerArray).addTo(this.map);
    }

    drawWalkRoute(coordinates, source, dest) {
        if (this.layers.walk) this.map.removeLayer(this.layers.walk);
        const walkLayers = [];

        if (source && coordinates.length > 0) {
            const startLine = L.polyline([[source.lat, source.lng], [coordinates[0][1], coordinates[0][0]]], {
                color: '#33ccff',
                weight: 3,
                dashArray: '5, 8',
                opacity: 0.6
            });
            walkLayers.push(startLine);
        }

        if (dest && coordinates.length > 0) {
            const last = coordinates[coordinates.length - 1];
            const endLine = L.polyline([[dest.lat, dest.lng], [last[1], last[0]]], {
                color: '#ff5252',
                weight: 3,
                dashArray: '5, 8',
                opacity: 0.6
            });
            walkLayers.push(endLine);
        }

        this.layers.walk = L.layerGroup(walkLayers).addTo(this.map);
    }

    drawEndpointMarkers(source, dest) {
        if (this.layers.source) this.map.removeLayer(this.layers.source);
        if (this.layers.dest) this.map.removeLayer(this.layers.dest);

        if (source) {
            this.layers.source = L.circleMarker([source.lat, source.lng], {
                radius: 12,
                fillColor: '#33ccff',
                fillOpacity: 1,
                color: 'white',
                weight: 4,
            }).addTo(this.map).bindPopup(`<b>Start:</b> ${source.name.split(',')[0]}`);
        }

        if (dest) {
            this.layers.dest = L.circleMarker([dest.lat, dest.lng], {
                radius: 12,
                fillColor: '#ff3d00',
                fillOpacity: 1,
                color: 'white',
                weight: 4,
            }).addTo(this.map).bindPopup(`<b>End:</b> ${dest.name.split(',')[0]}`);
        }
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
        this.refreshRouteInspector();
    }

    refreshRouteInspector() {
        if (this.layers.debugRoute) {
            this.map.removeLayer(this.layers.debugRoute);
            this.layers.debugRoute = null;
        }
        this.routeInspector.edgeLines = [];

        if (!this.routeInspector.enabled || !this.routeInspector.routeObj) {
            return;
        }

        const { routeObj } = this.routeInspector;
        const layers = [];
        const edgeLines = [];

        routeObj.legs.forEach((leg, index) => {
            const fromStep = routeObj.steps[index];
            const toStep = routeObj.steps[index + 1];
            const line = L.polyline([
                [leg.from_node[1], leg.from_node[0]],
                [leg.to_node[1], leg.to_node[0]],
            ], {
                color: '#007aff',
                weight: 5,
                opacity: 0.95,
                lineCap: 'round',
            });

            line.on('click', () => {
                edgeLines.forEach((edgeLine) => edgeLine.setStyle({ color: '#007aff', weight: 5 }));
                line.setStyle({ color: '#ff3d00', weight: 7 });
                if (this.routeInspector.onEdgeClick) {
                    this.routeInspector.onEdgeClick({
                        index,
                        edgeId: leg.edge_id,
                        distanceM: leg.base_length,
                        durationSec: leg.duration_sec,
                        speedLimitKmh: leg.speed_limit,
                        fromNodeId: fromStep?.node_id ?? 0,
                        toNodeId: toStep?.node_id ?? 0,
                        fromLat: fromStep?.lat || leg.from_node[1],
                        fromLon: fromStep?.lon || leg.from_node[0],
                        toLat: toStep?.lat || leg.to_node[1],
                        toLon: toStep?.lon || leg.to_node[0],
                    });
                }
            });

            edgeLines.push(line);
            layers.push(line);
        });

        routeObj.steps.forEach((step, index) => {
            layers.push(L.circleMarker([step.lat, step.lon], {
                radius: index === 0 || index === routeObj.steps.length - 1 ? 5 : 3,
                fillColor: index === 0 ? '#33ccff' : index === routeObj.steps.length - 1 ? '#ff3d00' : '#ffffff',
                fillOpacity: 1,
                color: '#0f172a',
                weight: 1.5,
            }));
        });

        this.routeInspector.edgeLines = edgeLines;
        this.layers.debugRoute = L.layerGroup(layers).addTo(this.map);
    }

    clearMapLayers() {
        Object.values(this.layers).forEach(layer => {
            if (layer && !(layer instanceof Map)) this.map.removeLayer(layer);
        });
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
        this.routeInspector.routeObj = null;
        this.routeInspector.edgeLines = [];
    }

    clearRouteLayers() {
        const routeLayerKeys = ['route', 'altRoutes', 'walk', 'source', 'dest', 'debugRoute'];
        routeLayerKeys.forEach((key) => {
            const layer = this.layers[key];
            if (!layer) return;
            this.map.removeLayer(layer);
            this.layers[key] = null;
        });
        this.routeInspector.routeObj = null;
        this.routeInspector.edgeLines = [];
    }

    initCarMarker(startPos) {
        const carIcon = L.divIcon({
            className: 'car-icon-container',
            html: `<div class="car-body"><img src="assets/car.svg" width="30" height="45"></div>`,
            iconSize: [30, 45],
            iconAnchor: [15, 22]
        });

        this.removeCarMarker();
        this.layers.car = L.marker(startPos, { icon: carIcon, zoomPanOptions: { animate: true } }).addTo(this.map);
    }

    updateCarPositionAndRotation(pos, bearing) {
        if (!this.layers.car) return;
        this.layers.car.setLatLng(pos);
        const carBody = this.layers.car.getElement().querySelector('.car-body');
        if (carBody) carBody.style.transform = `rotate(${bearing}deg)`;
    }

    removeCarMarker() {
        if (this.layers.car) {
            this.map.removeLayer(this.layers.car);
            this.layers.car = null;
        }
    }

    upsertDebugCar(id, pos) {
        if (!pos) return;
        let marker = this.layers.debugCars.get(id);
        if (!marker) {
            const debugCarIcon = L.divIcon({
                className: 'debug-car-icon-container',
                html: `<div class="debug-car-body"></div>`,
                iconSize: [14, 14],
                iconAnchor: [7, 7]
            });
            marker = L.marker(pos, { icon: debugCarIcon, interactive: false, zIndexOffset: 600 });
            if (this.debugCarsVisible) {
                marker.addTo(this.map);
            }
            this.layers.debugCars.set(id, marker);
            return;
        }
        marker.setLatLng(pos);
        if (this.debugCarsVisible && !this.map.hasLayer(marker)) {
            marker.addTo(this.map);
        }
    }

    removeDebugCar(id) {
        const marker = this.layers.debugCars.get(id);
        if (!marker) return;
        this.map.removeLayer(marker);
        this.layers.debugCars.delete(id);
    }

    clearDebugCars() {
        for (const marker of this.layers.debugCars.values()) {
            this.map.removeLayer(marker);
        }
        this.layers.debugCars.clear();
    }

    syncDebugCars(cars) {
        const liveIds = new Set();
        cars.forEach((car) => {
            if (!car?.id) return;
            liveIds.add(car.id);
            this.upsertDebugCar(car.id, [car.lat, car.lon]);
        });

        for (const [id, marker] of this.layers.debugCars.entries()) {
            if (liveIds.has(id)) continue;
            this.map.removeLayer(marker);
            this.layers.debugCars.delete(id);
        }
    }

    setDebugCarsVisible(visible) {
        this.debugCarsVisible = visible;
        for (const marker of this.layers.debugCars.values()) {
            const isOnMap = this.map.hasLayer(marker);
            if (visible && !isOnMap) {
                marker.addTo(this.map);
            } else if (!visible && isOnMap) {
                this.map.removeLayer(marker);
            }
        }
    }
}

export const mapInstance = new MapManager();
